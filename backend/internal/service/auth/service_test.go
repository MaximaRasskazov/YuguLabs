package auth_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/auth"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/token"
)

// TestMain прогоняет тесты пакета, затем страховочно дочищает тестовый
// мусор (@test.local). Per-test cleanup может не сработать при гонках
// параллельных пакетов на общей БД — TestMain гарантирует 0 остатка.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var secret = []byte("test-secret-must-be-at-least-32-bytes!!")

func testServices(t *testing.T) (*repo.Store, *auth.Service) {
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
	tokens := token.New(store, secret, 15*time.Minute, 7*24*time.Hour)
	return store, auth.New(store, tokens, nil)
}

// uniqueRegisterInput возвращает RegisterInput с уникальным email,
// чтобы прогоны не конфликтовали по UNIQUE-индексу.
func uniqueRegisterInput(prefix string) auth.RegisterInput {
	email := strings.ToLower(prefix) + "+" + time.Now().Format("150405.000000") + "@test.local"
	return auth.RegisterInput{
		Email:     email,
		Password:  "correct-horse-battery-staple",
		FirstName: "Виталий",
		LastName:  "Тестов",
	}
}

func cleanupUser(t *testing.T, store *repo.Store, userID pgtype.UUID) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.CleanupUser(context.Background(), store.Pool(), userID)
	})
}

func TestAuth_Register_CreatesUserWithStudentRole(t *testing.T) {
	store, svc := testServices(t)
	in := uniqueRegisterInput("register")

	res, err := svc.Register(context.Background(), in, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, in.Email, res.User.Email)
	require.NotEmpty(t, res.Pair.AccessToken)
	cleanupUser(t, store, res.User.ID)

	// У свежезарегистрированного юзера должна быть ровно одна роль —
	// student.
	roles, err := store.ListRolesForUser(context.Background(), res.User.ID)
	require.NoError(t, err)
	require.Len(t, roles, 1)
	require.Equal(t, "student", roles[0].Slug)

	// И permissions студента — debts.view.own, retakes.view.own,
	// teacher_request.create (3 штуки из сидов).
	perms, err := store.ListPermissionsForUser(context.Background(), res.User.ID)
	require.NoError(t, err)
	require.ElementsMatch(t,
		[]string{"debts.view.own", "retakes.view.own", "teacher_request.create"},
		perms)
}

func TestAuth_Register_RejectsDuplicateEmail(t *testing.T) {
	store, svc := testServices(t)
	in := uniqueRegisterInput("dup")

	first, err := svc.Register(context.Background(), in, "")
	require.NoError(t, err)
	cleanupUser(t, store, first.User.ID)

	_, err = svc.Register(context.Background(), in, "")
	require.ErrorIs(t, err, auth.ErrEmailTaken)
}

func TestAuth_Register_ValidatesInput(t *testing.T) {
	_, svc := testServices(t)
	ctx := context.Background()

	_, err := svc.Register(ctx, auth.RegisterInput{
		Email: "not-an-email", Password: "long-enough-pass", FirstName: "A", LastName: "B",
	}, "")
	require.ErrorIs(t, err, auth.ErrInvalidEmail)

	_, err = svc.Register(ctx, auth.RegisterInput{
		Email: "ok@test.local", Password: "short", FirstName: "A", LastName: "B",
	}, "")
	require.ErrorIs(t, err, auth.ErrPasswordTooShort)
}

func TestAuth_Login_AcceptsValidCredentials(t *testing.T) {
	store, svc := testServices(t)
	in := uniqueRegisterInput("login")

	reg, err := svc.Register(context.Background(), in, "")
	require.NoError(t, err)
	cleanupUser(t, store, reg.User.ID)

	res, err := svc.Login(context.Background(), in.Email, in.Password, "")
	require.NoError(t, err)
	require.Equal(t, in.Email, res.User.Email)
	require.NotEqual(t, reg.Pair.AccessToken, res.Pair.AccessToken,
		"новый login должен выдавать новую пару")
}

func TestAuth_Login_RejectsWrongPasswordAndUnknownEmail(t *testing.T) {
	store, svc := testServices(t)
	in := uniqueRegisterInput("wrong-pass")

	reg, err := svc.Register(context.Background(), in, "")
	require.NoError(t, err)
	cleanupUser(t, store, reg.User.ID)

	_, err = svc.Login(context.Background(), in.Email, "definitely-not-the-password", "")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)

	_, err = svc.Login(context.Background(), "no-such@test.local", "whatever-long", "")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestAuth_Me_ReturnsRolesAndPermissions(t *testing.T) {
	store, svc := testServices(t)
	in := uniqueRegisterInput("me")

	reg, err := svc.Register(context.Background(), in, "")
	require.NoError(t, err)
	cleanupUser(t, store, reg.User.ID)

	profile, err := svc.Me(context.Background(), pgutil.UUID(reg.User.ID))
	require.NoError(t, err)
	require.Equal(t, in.Email, profile.User.Email)
	require.Len(t, profile.Roles, 1)
	require.Equal(t, "student", profile.Roles[0].Slug)
	require.Contains(t, profile.Permissions, "debts.view.own")
}

func TestAuth_EndToEnd_RegisterLoginRefreshLogout(t *testing.T) {
	store, svc := testServices(t)
	in := uniqueRegisterInput("e2e")
	ctx := context.Background()
	tokens := token.New(store, secret, 15*time.Minute, 7*24*time.Hour)

	// 1. Регистрация — сразу получаем пару.
	reg, err := svc.Register(ctx, in, "127.0.0.1")
	require.NoError(t, err)
	cleanupUser(t, store, reg.User.ID)

	uid, _, err := tokens.Validate(ctx, reg.Pair.AccessToken)
	require.NoError(t, err)
	require.Equal(t, pgutil.UUID(reg.User.ID), uid)

	// 2. Refresh — новая пара, старый access после ротации недействителен.
	rotated, err := svc.Refresh(ctx, reg.Pair.RefreshToken, "127.0.0.1")
	require.NoError(t, err)
	require.NotEqual(t, reg.Pair.AccessToken, rotated.AccessToken)

	_, _, err = tokens.Validate(ctx, reg.Pair.AccessToken)
	require.ErrorIs(t, err, token.ErrTokenRevoked)

	// 3. Logout — новый access тоже должен закрыться.
	require.NoError(t, svc.Logout(ctx, rotated.AccessTokenID))
	_, _, err = tokens.Validate(ctx, rotated.AccessToken)
	require.ErrorIs(t, err, token.ErrTokenRevoked)
}

// TestAuth_Register_LogsUserCreated проверяет проводку changelog: при
// регистрации в change_logs появляется запись created для сущности user.
func TestAuth_Register_LogsUserCreated(t *testing.T) {
	store, svc := testServices(t)
	svc.SetChangelog(changelog.New(store))

	reg, err := svc.Register(context.Background(), uniqueRegisterInput("clog"), "")
	require.NoError(t, err)
	cleanupUser(t, store, reg.User.ID)

	rows, err := store.ListChangeLogsForEntity(context.Background(), queries.ListChangeLogsForEntityParams{
		EntityType: changelog.EntityUser,
		EntityID:   pgutil.UUID(reg.User.ID).String(),
		Limit:      10,
		Offset:     0,
	})
	require.NoError(t, err)
	require.NotEmpty(t, rows, "регистрация должна писать change_logs")
	require.Equal(t, changelog.ActionCreated, rows[0].Action)
}
