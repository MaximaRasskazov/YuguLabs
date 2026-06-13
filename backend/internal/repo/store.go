// Package repo предоставляет фасад над сгенерированным sqlc-кодом из
// internal/repo/queries и реализует Unit of Work для атомарных мутаций.
//
// Сервисы получают *Store через DI и используют либо встроенный
// *queries.Queries для одиночных запросов, либо Store.RunInTx для
// последовательностей, требующих транзакции (мутация сущности + запись
// в change_logs / audit_log).
package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
)

// Store — точка входа в слой доступа к данным.
//
// Встраивает *queries.Queries поверх пула соединений, поэтому все
// методы, сгенерированные sqlc, доступны напрямую как методы Store —
// например, store.GetUserByEmail(ctx, "u@example.com"). Внутри
// транзакции работа идёт через локальный *queries.Queries, который
// передаётся в RunInTx-callback.
type Store struct {
	*queries.Queries

	pool *pgxpool.Pool
}

// NewStore оборачивает уже открытый пул соединений.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		Queries: queries.New(pool),
		pool:    pool,
	}
}

// Pool возвращает нижележащий пул. Нужно тонкому кругу клиентов —
// например, healthcheck-у для Ping. Большинству сервисов pool не нужен.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// RunInTx выполняет fn внутри транзакции. Если fn возвращает ошибку
// (или сама транзакция не может закоммититься), изменения откатываются.
//
// Внутри fn нужно использовать переданный *queries.Queries — он
// прибинден к транзакции. Использование Store.Queries напрямую внутри
// fn создаст запись вне транзакции — это баг, не делать так.
//
// Пример:
//
//	err := store.RunInTx(ctx, func(q *queries.Queries) error {
//	    user, err := q.CreateUser(ctx, params)
//	    if err != nil { return err }
//	    _, err = q.CreateChangeLog(ctx, queries.CreateChangeLogParams{
//	        EntityType: "user", EntityID: user.ID.String(),
//	        Action: "created", After: afterJSON, CreatedBy: actorID,
//	    })
//	    return err
//	})
func (s *Store) RunInTx(ctx context.Context, fn func(*queries.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Rollback на закрытой транзакции возвращает pgx.ErrTxClosed —
	// это нормальное поведение, ошибку игнорируем (она именно про
	// "транзакция уже завершилась успешным Commit").
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(queries.New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// IsNotFound сообщает, является ли ошибка от sqlc-запроса
// "запись не найдена". pgx возвращает свою специфичную ошибку, и
// сервисы должны проверять её именно через этот хелпер, не импортируя
// pgx напрямую.
func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// DisciplineExists проверяет, существует ли дисциплина с данным id
// (включая soft-deleted записи). Нужно Restore, чтобы отличить
// "нет такой записи" от "запись активна и не требует восстановления".
func (s *Store) DisciplineExists(ctx context.Context, id pgtype.UUID) (bool, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM disciplines WHERE id = $1`, id).Scan(&n)
	return n > 0, err
}

// IsForeignKeyViolation сообщает, является ли err нарушением FOREIGN KEY
// (SQLSTATE 23503). Используется, чтобы отличить "referenced row not found"
// от других ошибок.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// GetLastSyncedAt возвращает метку времени последней успешной синхронизации.
func (s *Store) GetLastSyncedAt(ctx context.Context) (time.Time, error) {
	var t time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT last_synced_at FROM sync_state WHERE key = 'emulator'`,
	).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("get last_synced_at: %w", err)
	}
	return t, nil
}

// SetLastSyncedAt обновляет метку времени последней успешной синхронизации.
func (s *Store) SetLastSyncedAt(ctx context.Context, t time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sync_state SET last_synced_at = $1 WHERE key = 'emulator'`,
		t,
	)
	if err != nil {
		return fmt.Errorf("set last_synced_at: %w", err)
	}
	return nil
}

// UpsertDisciplineFromSync вставляет или обновляет дисциплину по external_id.
// Используется SyncService для idempotent-синхронизации.
func (s *Store) UpsertDisciplineFromSync(ctx context.Context, p UpsertDisciplineParams) error {
	// Пробуем сначала по external_id, затем по имени (для случаев когда
	// дисциплина была создана вручную до синхронизации).
	_, err := s.pool.Exec(ctx, `
		INSERT INTO disciplines (name, code, description, external_id, source, created_by)
		VALUES ($1, $2, $3, $4, 'sync', $5)
		ON CONFLICT (external_id) WHERE external_id IS NOT NULL
		DO UPDATE SET
			name        = EXCLUDED.name,
			code        = EXCLUDED.code,
			description = EXCLUDED.description,
			updated_at  = NOW()
	`, p.Name, p.Code, p.Description, p.ExternalID, p.SystemUserID)
	if err != nil {
		if !IsUniqueViolation(err, "idx_disciplines_name_lower") {
			return fmt.Errorf("upsert discipline: %w", err)
		}
		// Дисциплина с таким именем уже есть — обновляем external_id и код.
		_, err = s.pool.Exec(ctx, `
			UPDATE disciplines
			SET external_id = $1, code = $2, description = $3, updated_at = NOW()
			WHERE LOWER(name) = LOWER($4) AND deleted_at IS NULL
		`, p.ExternalID, p.Code, p.Description, p.Name)
		if err != nil {
			return fmt.Errorf("upsert discipline by name: %w", err)
		}
	}
	return nil
}

// UpsertDisciplineParams — параметры для UpsertDisciplineFromSync.
type UpsertDisciplineParams struct {
	Name         string
	Code         string
	Description  *string
	ExternalID   string
	SystemUserID pgtype.UUID
}

// UpsertDebtFromSync вставляет или обновляет долг по external_id.
// При конфликте обновляет только изменяемые поля (статус, оценка).
func (s *Store) UpsertDebtFromSync(ctx context.Context, p UpsertDebtParams) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO debts (student_id, discipline_id, issued_by, external_id, source, notes, status, final_grade, graded_at, graded_by)
		VALUES ($1, $2, $3, $4, 'sync', $5, $6, $7, $8, $9)
		ON CONFLICT (external_id) WHERE external_id IS NOT NULL
		DO UPDATE SET
			status      = EXCLUDED.status,
			final_grade = EXCLUDED.final_grade,
			graded_at   = EXCLUDED.graded_at,
			graded_by   = EXCLUDED.graded_by,
			notes       = EXCLUDED.notes,
			updated_at  = NOW()
	`, p.StudentID, p.DisciplineID, p.IssuedBy, p.ExternalID,
		p.Notes, p.Status, p.FinalGrade, p.GradedAt, p.GradedBy)
	if err != nil {
		return fmt.Errorf("upsert debt: %w", err)
	}
	return nil
}

// UpsertDebtParams — параметры для UpsertDebtFromSync.
type UpsertDebtParams struct {
	StudentID    pgtype.UUID
	DisciplineID pgtype.UUID
	IssuedBy     pgtype.UUID
	ExternalID   string
	Notes        *string
	Status       string
	FinalGrade   *int32
	GradedAt     pgtype.Timestamptz
	GradedBy     pgtype.UUID
}

// UpsertUserFromSync вставляет или обновляет пользователя по email.
// Если пользователь с таким email уже есть (создан вручную) — привязывает
// external_id и обновляет имя. Возвращает внутренний UUID пользователя.
func (s *Store) UpsertUserFromSync(ctx context.Context, p UpsertUserParams) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, first_name, last_name, middle_name, external_id, group_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (LOWER(email))
		DO UPDATE SET
			external_id   = EXCLUDED.external_id,
			first_name    = EXCLUDED.first_name,
			last_name     = EXCLUDED.last_name,
			middle_name   = EXCLUDED.middle_name,
			-- group_name: эмулятор — источник правды для студентов. Но если
			-- в этом upsert'е группа не пришла (NULL, напр. преподаватель),
			-- не затираем уже сохранённое значение.
			group_name    = COALESCE(EXCLUDED.group_name, users.group_name),
			password_hash = CASE
				WHEN users.password_hash = '' THEN EXCLUDED.password_hash
				ELSE users.password_hash
			END,
			updated_at    = NOW()
		RETURNING id
	`, p.Email, p.PasswordHash, p.FirstName, p.LastName, p.MiddleName, p.ExternalID, p.GroupName).Scan(&id)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("upsert user from sync: %w", err)
	}
	return id, nil
}

// UpsertUserParams — параметры для UpsertUserFromSync.
type UpsertUserParams struct {
	Email        string
	FirstName    string
	LastName     string
	MiddleName   *string
	ExternalID   string
	PasswordHash string  // хеш из эмулятора; пустая строка — не менять существующий
	GroupName    *string // учебная группа (только у студентов); nil — не трогать существующее
}

// GetUserIDByExternalID возвращает внутренний UUID пользователя по external_id.
func (s *Store) GetUserIDByExternalID(ctx context.Context, externalID string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE external_id = $1`,
		externalID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgx.ErrNoRows
		}
		return pgtype.UUID{}, fmt.Errorf("get user by external_id: %w", err)
	}
	return id, nil
}

// GetDisciplineIDByExternalID возвращает внутренний UUID дисциплины по external_id.
func (s *Store) GetDisciplineIDByExternalID(ctx context.Context, externalID string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM disciplines WHERE external_id = $1 AND deleted_at IS NULL`,
		externalID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgx.ErrNoRows
		}
		return pgtype.UUID{}, fmt.Errorf("get discipline by external_id: %w", err)
	}
	return id, nil
}

// AssignRoleFromSync назначает роль из эмулятора ТОЛЬКО при первом импорте —
// когда у пользователя ещё нет НИ ОДНОЙ активной роли. Если роль уже есть
// (в т.ч. изменённая вручную администратором), sync её НЕ трогает.
//
// Почему так: иначе sync переутверждал бы роль из эмулятора на каждом цикле и
// перетирал ручные изменения ролей и их откаты (admin сделал student→teacher
// или откатил — а sync через 5 минут возвращал бы роль эмулятора). Правило
// «sync сеет начальную роль, дальше роли ведёт администратор» делает ручное
// управление авторитетным и устраняет «прыгающую» роль.
func (s *Store) AssignRoleFromSync(ctx context.Context, userID, systemUserID pgtype.UUID, roleSlug string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO role_user (user_id, role_id, created_by)
		SELECT $1, r.id, $2
		FROM roles r
		WHERE r.slug = $3 AND r.deleted_at IS NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM role_user ru
		    WHERE ru.user_id = $1 AND ru.deleted_at IS NULL
		  )
	`, userID, systemUserID, roleSlug)
	if err != nil {
		return fmt.Errorf("assign role %s from sync: %w", roleSlug, err)
	}
	return nil
}

// CountDisciplines возвращает количество дисциплин, СИНХРОНИЗИРОВАННЫХ
// с эмулятором (external_id IS NOT NULL). Используется sync.Service чтобы
// решить, нужен ли первичный полный импорт.
//
// Считаем ТОЛЬКО эмуляторные записи, а не все вообще — иначе dev-сиды
// или руками-созданные через UI дисциплины "мешали" бы sync-у:
// он бы видел "уже есть 6 дисциплин" и пропускал первый импорт из
// эмулятора, оставляя БД с одним только сид-набором.
func (s *Store) CountDisciplines(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM disciplines
		WHERE external_id IS NOT NULL
		  AND deleted_at IS NULL
	`).Scan(&n)
	return n, err
}

// CountSyncedUsers возвращает количество пользователей, импортированных
// из эмулятора. external_id IS NOT NULL — единственный надёжный признак
// эмуляторного источника. Прежняя эвристика (`password_hash != ”
// AND email NOT LIKE '%localhost%'`) ловила и dev-сидов (у них пароль
// захэширован), и leftover-тестовых `+timestamp@test.local`-юзеров,
// из-за чего sync-импорт ошибочно пропускался.
func (s *Store) CountSyncedUsers(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE external_id IS NOT NULL`).Scan(&n)
	return n, err
}

// CountSyncedStudentsWithoutGroup — сколько импортированных из эмулятора
// студентов ещё без group_name. Используется sync'ом для разового back-fill
// групп после внедрения feat/sync-student-groups: до него студенты были
// засинхронены с пустой группой, а /changes их не переобновит, пока они
// не изменятся в эмуляторе. external_id IS NOT NULL — признак sync-источника;
// JOIN role_user/roles ограничивает выборку студентами (у преподавателей
// группы нет, их NULL — норма и back-fill трогать не должен).
func (s *Store) CountSyncedStudentsWithoutGroup(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT u.id)
		FROM users u
		JOIN role_user ru ON ru.user_id = u.id AND ru.deleted_at IS NULL
		JOIN roles r      ON r.id = ru.role_id  AND r.deleted_at  IS NULL
		WHERE u.external_id IS NOT NULL
		  AND u.group_name IS NULL
		  AND r.slug = 'student'
	`).Scan(&n)
	return n, err
}

// CountDebts — количество долгов из эмулятора. См. CountDisciplines.
func (s *Store) CountDebts(ctx context.Context) (int64, error) {
	var n int64
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM debts
		WHERE external_id IS NOT NULL
		  AND deleted_at IS NULL
	`).Scan(&n)
	return n, err
}

// IsUniqueViolation сообщает, является ли err нарушением UNIQUE-ограничения
// (SQLSTATE 23505). Если constraintName непустой, дополнительно проверяется
// имя конкретного constraint/partial-индекса.
func IsUniqueViolation(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != "23505" {
		return false
	}
	return constraintName == "" || pgErr.ConstraintName == constraintName
}
