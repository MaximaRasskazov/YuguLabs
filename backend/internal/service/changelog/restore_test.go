package changelog_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
)

// seedNamedUser создаёт пользователя с заданным именем и возвращает sqlc-строку.
func seedNamedUser(t *testing.T, s *repo.Store, first, last string) queries.User {
	t.Helper()
	email := "restore+" + time.Now().Format("150405.000000000") + "@test.local"
	u, err := s.CreateUser(context.Background(), queries.CreateUserParams{
		Email:        email,
		PasswordHash: "$2a$10$placeholder",
		FirstName:    first,
		LastName:     last,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.CleanupUser(context.Background(), s.Pool(), u.ID) })
	return u
}

func hasRole(roles []queries.Role, slug string) bool {
	for _, r := range roles {
		if r.Slug == slug {
			return true
		}
	}
	return false
}

// TestRestore_UserProfile — undo правки профиля возвращает прежнее имя и
// создаёт запись restored_from_log.
func TestRestore_UserProfile(t *testing.T) {
	s := testStore(t)
	svc := changelog.New(s)
	ctx := context.Background()

	u := seedNamedUser(t, s, "Старая", "Фамилия")
	actor := pgutil.UUID(u.ID)
	uid := actor.String()

	// Симулируем правку: меняем имя и логируем updated (before→after).
	newFirst := "Новая"
	_, err := s.UpdateUserProfile(ctx, queries.UpdateUserProfileParams{ID: u.ID, FirstName: &newFirst})
	require.NoError(t, err)
	require.NoError(t, s.RunInTx(ctx, func(q *queries.Queries) error {
		return svc.LogUpdatedTx(ctx, q, changelog.EntityUser, uid,
			changelog.Fields{"first_name": "Старая"},
			changelog.Fields{"first_name": "Новая"}, actor)
	}))

	rows, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 10, 0)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	logID := rows[0].ID

	// Откат.
	require.NoError(t, svc.RestoreFromLog(ctx, logID, actor))

	got, err := s.GetUserByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "Старая", got.FirstName, "профиль должен вернуться к состоянию before")

	rows2, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 10, 0)
	require.NoError(t, err)
	require.Equal(t, changelog.ActionRestoredFromLog, rows2[0].Action, "откат тоже логируется")
}

// TestRestore_RoleAssignUndo — undo выдачи роли снимает её.
func TestRestore_RoleAssignUndo(t *testing.T) {
	s := testStore(t)
	svc := changelog.New(s)
	ctx := context.Background()

	u := seedNamedUser(t, s, "Роль", "Тестов")
	actor := pgutil.UUID(u.ID)
	uid := actor.String()

	role, err := s.GetRoleBySlug(ctx, "teacher")
	require.NoError(t, err)
	rid := pgutil.UUID(role.ID).String()

	// Выдаём роль и логируем role_assigned (как делает rbac.AssignRole).
	require.NoError(t, s.RunInTx(ctx, func(q *queries.Queries) error {
		if _, err := q.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
			UserID: u.ID, RoleID: role.ID, CreatedBy: u.ID,
		}); err != nil {
			return err
		}
		return svc.LogRoleChangeTx(ctx, q, changelog.ActionRoleAssigned, uid, rid, role.Slug, role.Name, actor)
	}))

	roles, err := s.ListRolesForUser(ctx, u.ID)
	require.NoError(t, err)
	require.True(t, hasRole(roles, "teacher"))

	rows, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 10, 0)
	require.NoError(t, err)
	require.Equal(t, changelog.ActionRoleAssigned, rows[0].Action)

	// Откат → роль снята.
	require.NoError(t, svc.RestoreFromLog(ctx, rows[0].ID, actor))
	roles2, err := s.ListRolesForUser(ctx, u.ID)
	require.NoError(t, err)
	require.False(t, hasRole(roles2, "teacher"), "после отката роль должна быть снята")
}

// TestRestore_Unsupported — откат «created» не поддержан.
func TestRestore_Unsupported(t *testing.T) {
	s := testStore(t)
	svc := changelog.New(s)
	ctx := context.Background()

	u := seedNamedUser(t, s, "Не", "Откатить")
	actor := pgutil.UUID(u.ID)
	uid := actor.String()

	require.NoError(t, s.RunInTx(ctx, func(q *queries.Queries) error {
		return svc.LogCreatedTx(ctx, q, changelog.EntityUser, uid, changelog.UserSnapshot(u), actor)
	}))
	rows, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 10, 0)
	require.NoError(t, err)

	err = svc.RestoreFromLog(ctx, rows[0].ID, actor)
	require.ErrorIs(t, err, changelog.ErrRestoreUnsupported)
}

// TestRestore_NotFound — несуществующая запись лога.
func TestRestore_NotFound(t *testing.T) {
	s := testStore(t)
	svc := changelog.New(s)
	actor := seedActor(t, s) // из service_test.go, тот же пакет

	err := svc.RestoreFromLog(context.Background(), 999999999, actor)
	require.ErrorIs(t, err, changelog.ErrLogNotFound)
}

// TestRestore_RoleChangedUndo — откат атомарной смены роли возвращает
// прежнюю единственную роль: teacher→dean, откат → снова teacher (dean снят).
func TestRestore_RoleChangedUndo(t *testing.T) {
	s := testStore(t)
	svc := changelog.New(s)
	ctx := context.Background()

	u := seedNamedUser(t, s, "Смена", "Роли")
	actor := pgutil.UUID(u.ID)
	uid := actor.String()

	teacher, err := s.GetRoleBySlug(ctx, "teacher")
	require.NoError(t, err)
	dean, err := s.GetRoleBySlug(ctx, "dean")
	require.NoError(t, err)

	// Состояние «после смены teacher→dean»: активна dean + лог role_changed.
	require.NoError(t, s.RunInTx(ctx, func(q *queries.Queries) error {
		if _, err := q.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
			UserID: u.ID, RoleID: dean.ID, CreatedBy: u.ID,
		}); err != nil {
			return err
		}
		return svc.LogRoleChangedTx(ctx, q, uid, teacher.Slug, teacher.Name, dean.Slug, dean.Name, actor)
	}))

	rows, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 10, 0)
	require.NoError(t, err)
	require.Equal(t, changelog.ActionRoleChanged, rows[0].Action)

	require.NoError(t, svc.RestoreFromLog(ctx, rows[0].ID, actor))

	roles, err := s.ListRolesForUser(ctx, u.ID)
	require.NoError(t, err)
	require.True(t, hasRole(roles, "teacher"), "после отката должна вернуться teacher")
	require.False(t, hasRole(roles, "dean"), "после отката dean должна быть снята")
}

// TestRestore_RoleDoubleIsNoop — повторный откат той же записи о выдаче роли
// не падает на конфликте уникального индекса, а возвращает ErrRestoreNoop
// (роль уже снята — откат уже выполнен).
func TestRestore_RoleDoubleIsNoop(t *testing.T) {
	s := testStore(t)
	svc := changelog.New(s)
	ctx := context.Background()

	u := seedNamedUser(t, s, "Двойной", "Откат")
	actor := pgutil.UUID(u.ID)
	uid := actor.String()

	role, err := s.GetRoleBySlug(ctx, "teacher")
	require.NoError(t, err)
	rid := pgutil.UUID(role.ID).String()

	require.NoError(t, s.RunInTx(ctx, func(q *queries.Queries) error {
		if _, err := q.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
			UserID: u.ID, RoleID: role.ID, CreatedBy: u.ID,
		}); err != nil {
			return err
		}
		return svc.LogRoleChangeTx(ctx, q, changelog.ActionRoleAssigned, uid, rid, role.Slug, role.Name, actor)
	}))

	rows, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 10, 0)
	require.NoError(t, err)
	logID := rows[0].ID

	// Первый откат снимает роль…
	require.NoError(t, svc.RestoreFromLog(ctx, logID, actor))
	// …повторный откат той же записи — уже нечего снимать.
	require.ErrorIs(t, svc.RestoreFromLog(ctx, logID, actor), changelog.ErrRestoreNoop)
}

// TestRestore_RoleChangedDoubleIsNoop — точный кейс из бага: повторный откат
// записи о СМЕНЕ роли (role_changed) после того, как состояние уже целевое,
// возвращает ErrRestoreNoop и НЕ создаёт пустую restored_from_log-запись.
func TestRestore_RoleChangedDoubleIsNoop(t *testing.T) {
	s := testStore(t)
	svc := changelog.New(s)
	ctx := context.Background()

	u := seedNamedUser(t, s, "Повтор", "Смены")
	actor := pgutil.UUID(u.ID)
	uid := actor.String()

	teacher, err := s.GetRoleBySlug(ctx, "teacher")
	require.NoError(t, err)
	dean, err := s.GetRoleBySlug(ctx, "dean")
	require.NoError(t, err)

	require.NoError(t, s.RunInTx(ctx, func(q *queries.Queries) error {
		if _, err := q.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
			UserID: u.ID, RoleID: dean.ID, CreatedBy: u.ID,
		}); err != nil {
			return err
		}
		return svc.LogRoleChangedTx(ctx, q, uid, teacher.Slug, teacher.Name, dean.Slug, dean.Name, actor)
	}))

	rows, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 50, 0)
	require.NoError(t, err)
	logID := rows[0].ID

	// Первый откат применяется и пишет restored_from_log.
	require.NoError(t, svc.RestoreFromLog(ctx, logID, actor))
	after1, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 50, 0)
	require.NoError(t, err)

	// Повторный откат: состояние уже целевое → noop без записи в лог.
	require.ErrorIs(t, svc.RestoreFromLog(ctx, logID, actor), changelog.ErrRestoreNoop)
	after2, err := svc.ListForEntity(ctx, changelog.EntityUser, uid, 50, 0)
	require.NoError(t, err)
	require.Equal(t, len(after1), len(after2), "повторный (пустой) откат не должен добавлять записи в лог")
}
