package repo_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
)

// TestMain прогоняет тесты пакета, затем страховочно дочищает тестовый
// мусор (@test.local). Per-test cleanup может не сработать при гонках
// параллельных пакетов на общей БД — TestMain гарантирует 0 остатка.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// testStore возвращает Store, подключённый к БД из TEST_DATABASE_URL.
// Если переменная не задана — тест скипается (это интеграционный тест,
// требующий поднятый postgres c накатанными миграциями).
func testStore(t *testing.T) *repo.Store {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))

	t.Cleanup(pool.Close)
	return repo.NewStore(pool)
}

// uniqueEmail возвращает email, уникальный для каждого вызова теста, чтобы
// прогоны не конфликтовали по UNIQUE-индексу idx_users_email_lower.
func uniqueEmail(prefix string) string {
	return strings.ToLower(prefix) + "+" + time.Now().Format("150405.000000") + "@test.local"
}

func newUserParams(email string) queries.CreateUserParams {
	return queries.CreateUserParams{
		Email:        email,
		PasswordHash: "$2a$10$placeholder",
		FirstName:    "Test",
		LastName:     "User",
	}
}

// systemUUID — служебный пользователь (created_by для sync-операций).
var systemUUID = pgtype.UUID{Bytes: uuid.MustParse("00000000-0000-0000-0000-000000000000"), Valid: true}

func activeRoleSlugs(t *testing.T, s *repo.Store, uid pgtype.UUID) []string {
	t.Helper()
	roles, err := s.ListRolesForUser(context.Background(), uid)
	require.NoError(t, err)
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.Slug)
	}
	return out
}

// TestAssignRoleFromSync_FirstImportOnly проверяет, что sync назначает роль
// из эмулятора только при первом импорте (когда активной роли ещё нет) и НЕ
// перетирает роль, установленную вручную. Без этого ломался откат смены роли:
// sync возвращал бы роль эмулятора после каждого undo.
func TestAssignRoleFromSync_FirstImportOnly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	u, err := s.CreateUser(ctx, newUserParams(uniqueEmail("syncrole")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.CleanupUser(ctx, s.Pool(), u.ID) })

	// 1. Первый импорт: ролей нет → роль назначается.
	require.NoError(t, s.AssignRoleFromSync(ctx, u.ID, systemUUID, "student"))
	require.ElementsMatch(t, []string{"student"}, activeRoleSlugs(t, s, u.ID))

	// 2. Повторный sync с ДРУГОЙ ролью эмулятора: активная роль уже есть →
	//    sync НЕ трогает (ручное управление авторитетно).
	require.NoError(t, s.AssignRoleFromSync(ctx, u.ID, systemUUID, "teacher"))
	require.ElementsMatch(t, []string{"student"}, activeRoleSlugs(t, s, u.ID),
		"sync не должен переутверждать роль, если у пользователя уже есть активная")

	// 3. Если активной роли не осталось — sync снова сеет роль.
	student, err := s.GetRoleBySlug(ctx, "student")
	require.NoError(t, err)
	require.NoError(t, s.DetachRoleFromUser(ctx, queries.DetachRoleFromUserParams{
		UserID: u.ID, RoleID: student.ID, DeletedBy: systemUUID,
	}))
	require.NoError(t, s.AssignRoleFromSync(ctx, u.ID, systemUUID, "teacher"))
	require.ElementsMatch(t, []string{"teacher"}, activeRoleSlugs(t, s, u.ID))
}

func TestStore_RunInTx_CommitsOnSuccess(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := uniqueEmail("commit")

	err := s.RunInTx(ctx, func(q *queries.Queries) error {
		_, err := q.CreateUser(ctx, newUserParams(email))
		return err
	})
	require.NoError(t, err)

	// После коммита запись должна быть видна вне транзакции.
	got, err := s.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	require.Equal(t, email, got.Email)

	// Cleanup: удаляем созданного пользователя со всеми зависимостями,
	// чтобы не накапливать мусор в dev-БД между прогонами.
	t.Cleanup(func() {
		_ = repo.CleanupUser(ctx, s.Pool(), got.ID)
	})
}

func TestStore_RunInTx_RollsBackOnError(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	email := uniqueEmail("rollback")
	sentinel := errors.New("откатить, бизнес-инвариант не сошёлся")

	err := s.RunInTx(ctx, func(q *queries.Queries) error {
		if _, err := q.CreateUser(ctx, newUserParams(email)); err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	// После rollback запись не должна существовать.
	_, err = s.GetUserByEmail(ctx, email)
	require.Error(t, err)
	require.True(t, repo.IsNotFound(err), "ожидалось IsNotFound, получили %v", err)
}

func TestStore_RunInTx_NestedQueriesShareTx(t *testing.T) {
	// Гарантия: несколько мутаций в одном RunInTx видят друг друга и
	// атомарно коммитятся вместе. Если rollback на любом этапе —
	// откатывается весь набор.
	s := testStore(t)
	ctx := context.Background()
	email := uniqueEmail("nested")

	var createdID pgtype.UUID

	err := s.RunInTx(ctx, func(q *queries.Queries) error {
		user, err := q.CreateUser(ctx, newUserParams(email))
		if err != nil {
			return err
		}
		createdID = user.ID

		// Чтение внутри той же транзакции должно вернуть только что
		// созданную запись.
		got, err := q.GetUserByEmail(ctx, email)
		if err != nil {
			return err
		}
		if got.Email != email {
			return errors.New("чтение внутри tx вернуло не ту запись")
		}
		return nil
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = repo.CleanupUser(ctx, s.Pool(), createdID)
	})
}

func TestIsNotFound(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_, err := s.GetUserByEmail(ctx, "definitely-not-existing@nowhere.invalid")
	require.Error(t, err)
	require.True(t, repo.IsNotFound(err))

	require.False(t, repo.IsNotFound(nil))
	require.False(t, repo.IsNotFound(errors.New("какая-то другая ошибка")))
}
