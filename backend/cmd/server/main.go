// Command server — точка входа HTTP-сервиса академических задолженностей.
//
// Старт: загружает конфиг, открывает пул соединений к Postgres, собирает
// слои (Store → TokenService → AuthService) и chi-роутер с middleware
// и маршрутами /api/auth/*. Корректно завершает работу по SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "github.com/MaximaRasskazov/Academic-debt-system/backend/docs"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/config"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/emulator"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/attendance"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/audit"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/auth"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changerequest"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/debt"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/deploy"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/discipline"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/notify"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/photo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/rbac"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/report"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/retake"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/retakerequest"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/scheduler"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/statement"
	syncsvc "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/sync"
	teacherrequest "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/teacher_request"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/token"
	usersvc "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/user"
	httpx "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http"
	mw "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/transport/http/middleware"
)

const (
	dbConnectTimeout  = 10 * time.Second
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	setupLogger()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config load: %w", err)
	}

	pool, err := newPool(cfg)
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer pool.Close()
	slog.Info("database connected", "host", cfg.DBHost, "db", cfg.DBName)

	// Клиент эмулятора создаём один раз и переиспользуем в auth (proxy-login)
	// и sync. Если EMULATOR_URL пуст — клиент nil, эмулятор-логин отключён.
	var emulatorClient *emulator.Client
	if cfg.EmulatorURL != "" {
		emulatorClient = emulator.New(cfg.EmulatorURL, cfg.EmulatorAPIKey)
	}

	store := repo.NewStore(pool)
	tokens := token.New(store, []byte(cfg.JWTSecret), cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	authSvc := auth.New(store, tokens, emulatorClient)
	rbacSvc := rbac.New(store)
	auditSvc := audit.New(store)
	changelogSvc := changelog.New(store)
	// История мутаций users/roles: auth и rbac пишут в change_logs.
	authSvc.SetChangelog(changelogSvc)
	rbacSvc.SetChangelog(changelogSvc)
	// Фотографии профиля (лаба «загрузка файлов»): сжатый оригинал + аватар
	// 128×128 в БД, защищённое скачивание, админский ZIP+Excel-архив.
	// Действия с фото логируются в change_logs.
	photoSvc := photo.New(store)
	photoSvc.SetChangelog(changelogSvc)
	disciplineSvc := discipline.New(store, auditSvc, changelogSvc)
	reportSvc := report.New(store)

	// Авто-зачёт по файлу успеваемости (лаба №12). Без БД: на вход .xlsx,
	// на выход JSON. Правила зачёта берутся из конфигурации.
	attendanceSvc := attendance.New(cfg.RequiredLabs, cfg.AttendanceThreshold)

	usersSvc := usersvc.New(store)
	// Админская правка профиля (PATCH /api/users/{id}) пишется в change_logs
	// с автором-администратором.
	usersSvc.SetChangelog(changelogSvc)

	// Webhook авто-деплоя (лаба №6). Без БД: запускает git-команды в
	// GIT_REPO_PATH под in-process блокировкой и пишет журнал деплоя.
	deploySvc := deploy.New(
		deploy.Config{
			RepoPath: cfg.GitRepoPath,
			Branch:   cfg.GitDefaultBranch,
			Timeout:  cfg.GitDeployTimeout,
			LockTTL:  cfg.GitDeployTimeout,
		},
		deploy.NewGitRunner(),
		deploy.NewMemoryLock(),
		deploy.NewFileRecorder(cfg.GitDeployLog),
	)

	notifyHub := notify.NewHub()
	notifySvc := notify.NewService(store, notifyHub, notify.EmailConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUser,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	})
	// Закрываем email-воркер до pool.Close(): воркер может делать запросы
	// к БД (GetUserByID для адреса), пул должен быть жив.
	defer notifySvc.Close()

	// teacherRequestSvc создаём после notifySvc — он шлёт автору заявки
	// письмо о решении (approved/rejected).
	teacherRequestSvc := teacherrequest.New(store, rbacSvc, notifySvc)

	retakeSvc := retake.New(store, auditSvc, changelogSvc, notifySvc)
	debtSvc := debt.New(store, auditSvc, changelogSvc, disciplineSvc, notifySvc)
	changeRequestSvc := changerequest.New(store, auditSvc, changelogSvc, notifySvc)
	retakeRequestSvc := retakerequest.New(store, auditSvc, changelogSvc, notifySvc)
	statementSvc := statement.New(store, auditSvc, notifySvc)

	// Обратный sync оценок в эмулятор (write-back). Подключаем тот же
	// клиент, что и для proxy-login/sync. Если EMULATOR_URL пуст —
	// emulatorClient == nil, и сервисы оставляют write-back выключенным.
	if emulatorClient != nil {
		debtSvc.SetGradeSender(emulatorClient)
		statementSvc.SetGradeSender(emulatorClient)
	}

	// Шедулер автопереходов retake-статусов крутится параллельно
	// HTTP-серверу. Останавливаем его через schedCancel перед
	// shutdown, чтобы не словить race на повисшем UPDATE при закрытии
	// пула соединений.
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	schedSvc := scheduler.New(store, auditSvc, scheduler.DefaultInterval)
	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		_ = schedSvc.Run(schedCtx)
	}()

	// Фоновая синхронизация с эмулятором деканата.
	// Запускается и подключается к API только если задан EMULATOR_URL.
	// syncSvc остаётся nil-интерфейсом когда URL не задан — хендлер
	// вернёт 503, что явно сообщает о том, что фича отключена.
	var syncSvc *syncsvc.Service
	if cfg.EmulatorURL != "" {
		// pgutil.PgUUID превращает uuid.Nil в NULL — нельзя использовать
		// для системного пользователя, чей UUID намеренно равен нулевому.
		systemUserID := pgtype.UUID{Bytes: uuid.MustParse("00000000-0000-0000-0000-000000000000"), Valid: true}
		syncSvc = syncsvc.New(store, emulatorClient, systemUserID)

		syncCtx, syncCancel := context.WithCancel(context.Background())
		syncDone := make(chan struct{})
		go func() {
			defer close(syncDone)
			slog.Info("sync: планировщик запущен", "interval", cfg.SyncInterval)

			// Первый sync — СРАЗУ после старта, не ждём первого тика
			// time.Ticker. Без этого свежеподнятый контейнер 5 минут стоит
			// с одними сидами, пользователи смотрят в админку и не видят
			// данных из эмулятора. Все последующие проходят по тикеру.
			if err := syncSvc.Sync(syncCtx); err != nil {
				slog.Warn("sync: первый запуск завершился с ошибкой", "err", err)
			}

			ticker := time.NewTicker(cfg.SyncInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := syncSvc.Sync(syncCtx); err != nil {
						slog.Warn("sync: ошибка синхронизации", "err", err)
					}
				case <-syncCtx.Done():
					return
				}
			}
		}()
		defer func() { syncCancel(); <-syncDone }()
	}

	handler := httpx.NewRouter(httpx.Deps{
		Cfg:             cfg,
		Pool:            pool,
		Auth:            authSvc,
		Tokens:          tokens,
		RBAC:            rbacSvc,
		Disciplines:     disciplineSvc,
		Debts:           debtSvc,
		Retakes:         retakeSvc,
		Statements:      statementSvc,
		ChangeRequests:  changeRequestSvc,
		RetakeRequests:  retakeRequestSvc,
		Reports:         reportSvc,
		Notify:          notifySvc,
		NotifyHub:       notifyHub,
		TeacherRequests: teacherRequestSvc,
		Sync:            syncSvc,
		Users:           usersSvc,
		Changelog:       changelogSvc,
		Deploy:          deploySvc,
		Photos:          photoSvc,
		Attendance:      attendanceSvc,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	if err := runServer(srv, cfg); err != nil {
		return err
	}

	// Сервер уже остановлен — гасим шедулер и ждём пока он закроется,
	// прежде чем pool.Close() в defer.
	schedCancel()
	<-schedDone
	return nil
}

func setupLogger() {
	// Используем middleware-обёртку: JSONHandler + автоматическое
	// добавление request_id и user_id из context. Логгер общий через
	// slog.SetDefault, поэтому слою кода нужно просто звать
	// slog.InfoContext(ctx, ...) и trace-id попадёт в JSON-вывод.
	mw.SetupContextLogger(slog.LevelInfo)
}

func newPool(cfg *config.Config) (*pgxpool.Pool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbConnectTimeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL())
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

// runServer запускает сервер и ждёт SIGINT/SIGTERM для graceful shutdown.
// Возвращает nil, если сервер корректно остановлен, или ошибку, если не смог.
func runServer(srv *http.Server, cfg *config.Config) error {
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", srv.Addr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case sig := <-stop:
		slog.Info("shutdown signal received", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	slog.Info("server stopped cleanly")
	return nil
}
