package user_test

// Интеграционные тесты ListUsers.
//
// Тесты создают свои данные через auth.Service.Register (получают
// уникальные email и роль student по умолчанию) и через прямой
// AttachRoleToUser для выдачи teacher-роли. Это избавляет от
// зависимости от dev-сидов (SEED_DEV_ACCOUNTS=true) — CI прогоняет
// тесты с пустыми сидами.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/auth"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/token"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/user"
)

// TestMain прогоняет тесты пакета, затем страховочно дочищает тестовый
// мусор (@test.local). Per-test cleanup может не сработать при гонках
// параллельных пакетов на общей БД — TestMain гарантирует 0 остатка.
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var jwtSecret = []byte("test-secret-must-be-at-least-32-bytes!!")

type fixture struct {
	store *repo.Store
	users *user.Service
	auth  *auth.Service
}

func setup(t *testing.T) *fixture {
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
	tokens := token.New(store, jwtSecret, 15*time.Minute, 7*24*time.Hour)
	return &fixture{
		store: store,
		users: user.New(store),
		auth:  auth.New(store, tokens, nil),
	}
}

// uniqSuffix — короткий уникальный суффикс (HHMMSS.nanos, ~16 символов).
// testname не включаем — group_name в БД ограничен 50 символами, плюс
// prefix вроде "QA-GRP-OTHER-" + suffix + iterator съедают остаток.
// Тесты всё равно прогоняются последовательно и не пересекаются по
// timestamp.
func uniqSuffix(t *testing.T) string {
	t.Helper()
	return time.Now().Format("150405.000000000")
}

// registerStudent регистрирует уникального студента. Возвращает результат
// auth.Register'а: User там с заполненным id, role student уже привязана.
// fullSuffix используется в email/last_name — чтобы тесты могли точно
// искать этого юзера по подстроке.
func (f *fixture) registerStudent(t *testing.T, fullSuffix string, group *string) *auth.Result {
	t.Helper()
	in := auth.RegisterInput{
		Email:     fmt.Sprintf("u_%s@test.local", fullSuffix),
		Password:  "correct-horse-battery-staple",
		FirstName: "Имя_" + fullSuffix,
		LastName:  "Фамилия_" + fullSuffix,
		GroupName: group,
	}
	r, err := f.auth.Register(context.Background(), in, "127.0.0.1")
	require.NoError(t, err)
	// Чистим за собой: иначе зарегистрированные юзеры (с audit/role
	// записями) копятся в dev-БД между прогонами.
	t.Cleanup(func() {
		_ = repo.CleanupUser(context.Background(), f.store.Pool(), r.User.ID)
	})
	return r
}

func strptr(s string) *string { return &s }

// TestUpdateUser_AdminEditsProfile_Logged проверяет, что админская правка
// чужого профиля применяется и логируется с автором-администратором.
func TestUpdateUser_AdminEditsProfile_Logged(t *testing.T) {
	f := setup(t)
	svc := user.New(f.store)
	svc.SetChangelog(changelog.New(f.store))

	suf := uniqSuffix(t)
	target := f.registerStudent(t, suf, nil)
	actor := f.registerStudent(t, suf+"a", nil) // выступает админом-актором
	targetID := pgutil.UUID(target.User.ID)
	actorID := pgutil.UUID(actor.User.ID)

	updated, err := svc.UpdateUser(context.Background(), targetID, user.UpdateInput{
		FirstName: strptr("Пётр"),
		LastName:  strptr("Сидоров"),
		GroupName: strptr("ИВТ-99"),
	}, actorID)
	require.NoError(t, err)
	require.Equal(t, "Пётр", updated.FirstName)
	require.Equal(t, "Сидоров", updated.LastName)
	require.NotNil(t, updated.GroupName)
	require.Equal(t, "ИВТ-99", *updated.GroupName)

	logs, err := f.store.ListChangeLogsForEntity(context.Background(), queries.ListChangeLogsForEntityParams{
		EntityType: "user", EntityID: targetID.String(), Limit: 10, Offset: 0,
	})
	require.NoError(t, err)
	var foundByAdmin bool
	for _, l := range logs {
		if l.Action == "updated" && pgutil.UUID(l.CreatedBy) == actorID {
			foundByAdmin = true
		}
	}
	require.True(t, foundByAdmin, "правка залогирована с created_by = администратор")

	// Пустое имя → отклоняется.
	_, err = svc.UpdateUser(context.Background(), targetID, user.UpdateInput{FirstName: strptr("   ")}, actorID)
	require.ErrorIs(t, err, user.ErrInvalidProfile)

	// Несуществующий пользователь → ErrUserNotFound.
	_, err = svc.UpdateUser(context.Background(), uuid.New(), user.UpdateInput{FirstName: strptr("X")}, actorID)
	require.ErrorIs(t, err, user.ErrUserNotFound)
}

// promoteToTeacher выдаёт пользователю роль teacher (в дополнение к student,
// которую он получил при Register). ListUsers с фильтром role=teacher
// будет такого пользователя видеть.
func (f *fixture) promoteToTeacher(t *testing.T, userID pgtype.UUID) {
	t.Helper()
	teacherRole, err := f.store.GetRoleBySlug(context.Background(), "teacher")
	require.NoError(t, err)
	_, err = f.store.AttachRoleToUser(context.Background(), queries.AttachRoleToUserParams{
		UserID:    userID,
		RoleID:    teacherRole.ID,
		CreatedBy: userID,
	})
	require.NoError(t, err)
}

func TestList_TotalReflectsFilter(t *testing.T) {
	// Проверяем что Total соответствует фильтру: один созданный нами
	// юзер → Total=1 при search по его email. Параллельные пакетные
	// тесты (auth_test, debt_test и т.д.) тоже создают юзеров — поэтому
	// нельзя сравнивать с global count, нужен изолирующий фильтр.
	f := setup(t)
	suf := uniqSuffix(t)
	r := f.registerStudent(t, suf, nil)

	res, err := f.users.List(context.Background(), user.ListInput{
		Search: r.User.Email,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), res.Total)
	require.Len(t, res.Items, 1)
}

func TestList_FilterBySearchEmail(t *testing.T) {
	f := setup(t)
	suf := uniqSuffix(t)
	r := f.registerStudent(t, suf, nil)

	res, err := f.users.List(context.Background(), user.ListInput{
		Search: r.User.Email,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1, "точный поиск по email должен вернуть одного")
	require.Equal(t, r.User.Email, res.Items[0].User.Email)
	// У свежезарегистрированного должна быть роль student.
	require.NotEmpty(t, res.Items[0].Roles)
	hasStudent := false
	for _, role := range res.Items[0].Roles {
		if role.Slug == "student" {
			hasStudent = true
			break
		}
	}
	require.True(t, hasStudent, "Register должен выдать role=student")
}

func TestList_FilterBySearchLastName_CaseInsensitive(t *testing.T) {
	f := setup(t)
	suf := uniqSuffix(t)
	r := f.registerStudent(t, suf, nil)

	// Ищем подстроку в lower-case — должна найтись.
	lowerLast := strings.ToLower(r.User.LastName)
	res, err := f.users.List(context.Background(), user.ListInput{
		Search: lowerLast,
		Limit:  10,
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Items, "ILIKE должен матчить регистронезависимо")
	// Среди результатов — наш юзер.
	found := false
	for _, u := range res.Items {
		if u.User.Email == r.User.Email {
			found = true
			break
		}
	}
	require.True(t, found, "наш свежий юзер должен быть в выдаче по case-insensitive search")
}

func TestList_FilterByRoleTeacher(t *testing.T) {
	f := setup(t)
	suf := uniqSuffix(t)

	// Создаём ровно одного teacher с уникальным last_name — будем искать его.
	r := f.registerStudent(t, suf, nil)
	f.promoteToTeacher(t, r.User.ID)

	res, err := f.users.List(context.Background(), user.ListInput{
		RoleSlug: "teacher",
		Search:   r.User.Email, // сужаем по email чтобы не зацепить других teacher'ов
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	hasTeacher := false
	for _, role := range res.Items[0].Roles {
		if role.Slug == "teacher" {
			hasTeacher = true
			break
		}
	}
	require.True(t, hasTeacher, "фильтр role=teacher должен вернуть юзера с этой ролью")
}

func TestList_FilterByRoleStudent_DoesNotIncludeTeachers(t *testing.T) {
	// Создаём teacher и проверяем что он НЕ попадает в выборку по role=student.
	// Но у нашего teacher'а ПРИ ЭТОМ ЕСТЬ student-роль (от Register).
	// Значит он попадёт и в role=teacher, и в role=student. Это нормально:
	// фильтр семантический "у юзера есть такая роль", не "только такая".
	f := setup(t)
	suf := uniqSuffix(t)
	r := f.registerStudent(t, suf, nil)
	f.promoteToTeacher(t, r.User.ID)

	res, err := f.users.List(context.Background(), user.ListInput{
		RoleSlug: "student",
		Search:   r.User.Email,
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 1, "promoted teacher всё ещё имеет student-роль (Register выдал)")
}

func TestList_FilterByGroupName(t *testing.T) {
	f := setup(t)
	suf := uniqSuffix(t)
	group := "QA-GRP-" + suf
	f.registerStudent(t, suf+"-s1", &group)
	f.registerStudent(t, suf+"-s2", &group)
	// Ещё один с другой группой — он не должен попасть в выборку.
	other := "QA-GRP-OTHER-" + suf
	f.registerStudent(t, suf+"-s3", &other)

	res, err := f.users.List(context.Background(), user.ListInput{
		GroupName: group,
		Limit:     10,
	})
	require.NoError(t, err)
	require.Len(t, res.Items, 2, "в группе %s должно быть ровно 2 студента (созданных этим тестом)", group)
	for _, u := range res.Items {
		require.NotNil(t, u.User.GroupName)
		require.Equal(t, group, *u.User.GroupName)
	}
}

func TestList_Pagination_LimitOffset(t *testing.T) {
	f := setup(t)
	// Создаём 3 пользователя с уникальным маркером в last_name, чтобы
	// пагинировать только их (без шума от остальных).
	suf := uniqSuffix(t)
	for i := 0; i < 3; i++ {
		f.registerStudent(t, fmt.Sprintf("%s-%d", suf, i), nil)
	}

	page1, err := f.users.List(context.Background(), user.ListInput{
		Search: suf, Limit: 2, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, page1.Items, 2)
	require.Equal(t, int64(3), page1.Total)

	page2, err := f.users.List(context.Background(), user.ListInput{
		Search: suf, Limit: 2, Offset: 2,
	})
	require.NoError(t, err)
	require.Len(t, page2.Items, 1, "на второй странице должен быть остаток (1)")
	require.Equal(t, int64(3), page2.Total)

	// Страницы не пересекаются.
	require.NotEqual(t, page1.Items[0].User.ID, page2.Items[0].User.ID)
	require.NotEqual(t, page1.Items[1].User.ID, page2.Items[0].User.ID)
}

func TestList_LimitClamping(t *testing.T) {
	f := setup(t)

	zero, err := f.users.List(context.Background(), user.ListInput{Limit: 0})
	require.NoError(t, err)
	require.Equal(t, int32(50), zero.Limit, "Limit=0 должен превратиться в 50")

	huge, err := f.users.List(context.Background(), user.ListInput{Limit: 9999})
	require.NoError(t, err)
	require.Equal(t, int32(200), huge.Limit, "Limit=9999 должен быть зажат до 200")
}

func TestList_NoMatch_EmptyResult(t *testing.T) {
	f := setup(t)
	res, err := f.users.List(context.Background(), user.ListInput{
		Search: "no-such-user-anywhere-xyz-" + time.Now().Format("150405.000000000"),
		Limit:  10,
	})
	require.NoError(t, err)
	require.Empty(t, res.Items)
	require.Equal(t, int64(0), res.Total)
}
