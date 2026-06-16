// Package config загружает конфигурацию приложения из переменных окружения.
//
// При запуске Load() сначала пытается прочитать .env (если есть), затем
// собирает Config из ENV с дефолтами на dev-значения. Все обязательные
// поля валидируются перед возвратом — старт сервера должен падать рано,
// если конфигурация неполная, а не на первом запросе.
package config

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config — собранная конфигурация приложения. Хранится в памяти на время
// жизни процесса. Все поля read-only после Load().
type Config struct {
	// Application
	AppEnv   string
	AppPort  string
	LogLevel string

	// PostgreSQL
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string

	// JWT и сессии
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	// Cookie для refresh-токена
	CookieSecure   bool
	CookieDomain   string
	CookiePath     string
	CookieSameSite http.SameSite

	// CORS
	AllowedOrigins []string

	// SMTP — email-уведомления. Пустой SMTPHost отключает email-канал.
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	// Emulator — внешний эмулятор данных деканата.
	// Пустой EmulatorURL отключает фоновую синхронизацию.
	EmulatorURL    string
	EmulatorAPIKey string
	SyncInterval   time.Duration

	// Git-webhook авто-деплоя (лаба №6). Пустой GitWebhookSecret держит
	// эндпоинт /api/hooks/git выключенным (отвечает 503): без секрета
	// открытый деплой-хук — дыра в безопасности.
	GitWebhookSecret string
	GitDefaultBranch string
	GitRepoPath      string
	GitDeployLog     string
	GitDeployTimeout time.Duration

	// Attendance — параметры авто-зачёта по файлу успеваемости (лаба №12).
	// RequiredLabs — сколько лаб нужно сдать для зачёта.
	// AttendanceThreshold — минимальный процент посещаемости (0..100).
	// UploadMaxSizeMB — потолок размера загружаемого .xlsx в мегабайтах.
	RequiredLabs        int
	AttendanceThreshold int
	UploadMaxSizeMB     int
}

// Load читает .env (если есть) и собирает Config из ENV.
//
// Возвращает ошибку, если обязательные поля не заданы. В этом случае main
// должен сделать log.Fatal — работа без корректной конфигурации невозможна.
func Load() (*Config, error) {
	_ = godotenv.Load()

	accessTTL, err := parseDuration("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	refreshTTL, err := parseDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		AppPort:  getEnv("APP_PORT", "8080"),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBName:     getEnv("DB_NAME", "academic_debts"),
		DBUser:     getEnv("DB_USER", "academic"),
		DBPassword: getEnv("DB_PASSWORD", ""),

		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessTokenTTL:  accessTTL,
		RefreshTokenTTL: refreshTTL,

		CookieSecure:   parseBool("COOKIE_SECURE", false),
		CookieDomain:   getEnv("COOKIE_DOMAIN", ""),
		CookiePath:     getEnv("COOKIE_PATH", "/"),
		CookieSameSite: parseSameSite(getEnv("COOKIE_SAMESITE", "lax")),

		AllowedOrigins: parseList("ALLOWED_ORIGINS", []string{"http://localhost:5173"}),

		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     parseInt("SMTP_PORT", 1025),
		SMTPUser:     getEnv("SMTP_USER", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM", "noreply@localhost"),

		EmulatorURL:    getEnv("EMULATOR_URL", ""),
		EmulatorAPIKey: getEnv("EMULATOR_API_KEY", ""),
		SyncInterval:   parseDurationOrDefault("SYNC_INTERVAL", 5*time.Minute),

		GitWebhookSecret: getEnv("GIT_WEBHOOK_SECRET", ""),
		GitDefaultBranch: getEnv("GIT_DEFAULT_BRANCH", "main"),
		GitRepoPath:      getEnv("GIT_REPO_PATH", "."),
		GitDeployLog:     getEnv("GIT_DEPLOY_LOG", "storage/logs/deployment.log"),
		GitDeployTimeout: parseDurationOrDefault("GIT_DEPLOY_TIMEOUT", 5*time.Minute),

		RequiredLabs:        parseInt("REQUIRED_LABS", 5),
		AttendanceThreshold: parseInt("ATTENDANCE_PERCENT_THRESHOLD", 80),
		UploadMaxSizeMB:     parseInt("UPLOAD_MAX_SIZE_MB", 10),
	}

	if cfg.DBName == "" || cfg.DBUser == "" {
		return nil, fmt.Errorf("DB_NAME and DB_USER are required")
	}
	// JWT_SECRET — обязателен. Не разрешаем fallback на дефолт: это
	// security-issue (подделка токенов под предсказуемым ключом).
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be at least 32 bytes (got %d)", len(cfg.JWTSecret))
	}

	// Attendance-параметры должны быть положительными, иначе расчёт
	// процентов делил бы на ноль / выдавал бы бессмыслицу. Падаем на
	// старте, а не на первом запросе с файлом.
	if cfg.RequiredLabs <= 0 {
		return nil, fmt.Errorf("REQUIRED_LABS must be > 0 (got %d)", cfg.RequiredLabs)
	}
	if cfg.AttendanceThreshold < 0 || cfg.AttendanceThreshold > 100 {
		return nil, fmt.Errorf("ATTENDANCE_PERCENT_THRESHOLD must be in [0,100] (got %d)", cfg.AttendanceThreshold)
	}
	if cfg.UploadMaxSizeMB <= 0 {
		return nil, fmt.Errorf("UPLOAD_MAX_SIZE_MB must be > 0 (got %d)", cfg.UploadMaxSizeMB)
	}

	return cfg, nil
}

// DatabaseURL собирает DSN для подключения к Postgres.
// sslmode=disable достаточен для dev/локального docker-compose; для prod
// этот метод нужно расширить параметром или брать готовый URL из env.
func (c *Config) DatabaseURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

// IsProduction возвращает true, если приложение работает в production-режиме.
// Используется для feature-флагов (например, отключение debug-эндпоинтов).
func (c *Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func parseBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func parseDuration(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}

func parseSameSite(v string) http.SameSite {
	switch strings.ToLower(v) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func parseInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func parseDurationOrDefault(key string, fallback time.Duration) time.Duration {
	d, err := parseDuration(key, fallback)
	if err != nil {
		return fallback
	}
	return d
}

func parseList(key string, fallback []string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
