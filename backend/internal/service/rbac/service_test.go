package rbac_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/rbac"
)

// TestMain прогоняет тесты пакета, затем страховочно дочищает тестовый
// мусор (@test.local). Per-test cleanup может не сработать при гонках
// параллельных пакетов на общей БД — TestMain гарантирует 0 остатка.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func testRBAC(t *testing.T) (*repo.Store, *rbac.Service) {
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

	store := repo.NewStore(pool)
	return store, rbac.New(store)
}

// seedUser создаёт пользователя и возвращает его id (uuid.UUID).
func seedUser(t *testing.T, store *repo.Store, prefix string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	email := strings.ToLower(prefix) + "+" + time.Now().Format("150405.000000") + "@test.local"
	u, err := store.CreateUser(ctx, queries.CreateUserParams{
		Email:        email,
		PasswordHash: "$2a$10$placeholder",
		FirstName:    "T",
		LastName:     "U",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = repo.CleanupUser(ctx, store.Pool(), u.ID)
	})
	return pgutil.UUID(u.ID)
}

// attachRole напрямую привязывает роль к пользователю (минуя RBAC-сервис,
// чтобы не зависеть от его собственной проверки прав в тестах ниже).
func attachRole(t *testing.T, store *repo.Store, userID uuid.UUID, slug string) {
	t.Helper()
	ctx := context.Background()
	role, err := store.GetRoleBySlug(ctx, slug)
	require.NoError(t, err)
	_, err = store.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
		UserID:    pgutil.PgUUID(userID),
		RoleID:    role.ID,
		CreatedBy: pgutil.PgUUID(userID),
	})
	require.NoError(t, err)
}

func TestRBAC_HasPermission_StudentDefaults(t *testing.T) {
	store, svc := testRBAC(t)
	userID := seedUser(t, store, "perm-student")
	attachRole(t, store, userID, "student")
	ctx := context.Background()

	yes, err := svc.HasPermission(ctx, userID, "debts.view.own")
	require.NoError(t, err)
	require.True(t, yes, "у студента должен быть debts.view.own")

	no, err := svc.HasPermission(ctx, userID, "debts.view.all")
	require.NoError(t, err)
	require.False(t, no, "у студента не должно быть debts.view.all")
}

func TestRBAC_AssignRole_AdminCanGiveTeacher(t *testing.T) {
	store, svc := testRBAC(t)
	admin := seedUser(t, store, "rbac-admin")
	target := seedUser(t, store, "rbac-target")
	attachRole(t, store, admin, "admin")
	attachRole(t, store, target, "student")
	ctx := context.Background()

	require.NoError(t, svc.AssignRole(ctx, admin, target, "teacher"))

	roles, err := store.ListRolesForUser(ctx, pgutil.PgUUID(target))
	require.NoError(t, err)

	slugs := make([]string, 0, len(roles))
	for _, r := range roles {
		slugs = append(slugs, r.Slug)
	}
	require.Contains(t, slugs, "teacher")
}

func TestRBAC_AssignRole_DeanCannotGiveAdmin(t *testing.T) {
	// Privilege escalation: декан (level=700) не может выдать админа
	// (level=1000), даже имея permission roles.assign.
	store, svc := testRBAC(t)
	dean := seedUser(t, store, "rbac-dean")
	target := seedUser(t, store, "rbac-victim")
	attachRole(t, store, dean, "dean")
	ctx := context.Background()

	err := svc.AssignRole(ctx, dean, target, "admin")
	require.ErrorIs(t, err, rbac.ErrPrivilegeEscalation)

	// И не должно быть никаких записей в role_user, кроме изначальных.
	roles, err := store.ListRolesForUser(ctx, pgutil.PgUUID(target))
	require.NoError(t, err)
	for _, r := range roles {
		require.NotEqual(t, "admin", r.Slug)
	}
}

func TestRBAC_AssignRole_StudentDeniedWithoutPermission(t *testing.T) {
	// Студент не имеет roles.assign — даже если бы поднял прокладкой,
	// сервис должен вернуть ErrPermissionDenied.
	store, svc := testRBAC(t)
	student := seedUser(t, store, "rbac-stud")
	target := seedUser(t, store, "rbac-anyone")
	attachRole(t, store, student, "student")
	ctx := context.Background()

	err := svc.AssignRole(ctx, student, target, "teacher")
	require.ErrorIs(t, err, rbac.ErrPermissionDenied)
}

func TestRBAC_RevokeRole_AdminRemovesTeacher(t *testing.T) {
	store, svc := testRBAC(t)
	admin := seedUser(t, store, "rbac-rev-admin")
	target := seedUser(t, store, "rbac-rev-target")
	attachRole(t, store, admin, "admin")
	attachRole(t, store, target, "teacher")
	ctx := context.Background()

	require.NoError(t, svc.RevokeRole(ctx, admin, target, "teacher"))

	roles, err := store.ListRolesForUser(ctx, pgutil.PgUUID(target))
	require.NoError(t, err)
	for _, r := range roles {
		require.NotEqual(t, "teacher", r.Slug, "роль teacher должна была быть отозвана")
	}
}

func TestRBAC_AssignRole_UnknownRoleSlug(t *testing.T) {
	store, svc := testRBAC(t)
	admin := seedUser(t, store, "rbac-unk-admin")
	target := seedUser(t, store, "rbac-unk-target")
	attachRole(t, store, admin, "admin")

	err := svc.AssignRole(context.Background(), admin, target, "no-such-role")
	require.ErrorIs(t, err, rbac.ErrRoleNotFound)
}

func TestRBAC_ChangeRole_AtomicSwap(t *testing.T) {
	// Смена роли одним вызовом: новая выдана, старая снята.
	store, svc := testRBAC(t)
	admin := seedUser(t, store, "rbac-chg-admin")
	target := seedUser(t, store, "rbac-chg-target")
	attachRole(t, store, admin, "admin")
	attachRole(t, store, target, "student")
	ctx := context.Background()

	require.NoError(t, svc.ChangeRole(ctx, admin, target, "student", "teacher"))

	roles, err := store.ListRolesForUser(ctx, pgutil.PgUUID(target))
	require.NoError(t, err)
	slugs := make([]string, 0, len(roles))
	for _, r := range roles {
		slugs = append(slugs, r.Slug)
	}
	require.Contains(t, slugs, "teacher", "новая роль должна быть выдана")
	require.NotContains(t, slugs, "student", "старая роль должна быть снята")
}

func TestRBAC_ChangeRole_DeniedKeepsOldRole(t *testing.T) {
	// Если смена отклонена (privilege escalation) — не должно примениться
	// НИЧЕГО: старая роль остаётся, новая не добавляется.
	store, svc := testRBAC(t)
	dean := seedUser(t, store, "rbac-chg-dean")
	target := seedUser(t, store, "rbac-chg-keep")
	attachRole(t, store, dean, "dean")
	attachRole(t, store, target, "student")
	ctx := context.Background()

	err := svc.ChangeRole(ctx, dean, target, "student", "admin")
	require.ErrorIs(t, err, rbac.ErrPrivilegeEscalation)

	roles, err := store.ListRolesForUser(ctx, pgutil.PgUUID(target))
	require.NoError(t, err)
	slugs := make([]string, 0, len(roles))
	for _, r := range roles {
		slugs = append(slugs, r.Slug)
	}
	require.Contains(t, slugs, "student", "при отклонённой смене старая роль остаётся")
	require.NotContains(t, slugs, "admin")
}
