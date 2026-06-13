package changelog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
)

// Действия изменения ролей пользователя (для истории и undo).
const (
	ActionRoleAssigned = "role_assigned"
	ActionRoleRevoked  = "role_revoked"
	// ActionRoleChanged — атомарная смена роли (снятие старой + выдача новой)
	// одной записью: before — прежняя роль, after — новая. Так история
	// читается как «Роль изменена: X → Y», а откат возвращает прежнюю роль.
	ActionRoleChanged = "role_changed"
)

var (
	ErrLogNotFound        = errors.New("changelog: запись истории не найдена")
	ErrRestoreUnsupported = errors.New("changelog: откат для этого типа записи не поддержан")
	// ErrRestoreNoop — состояние уже соответствует записи: откат уже выполнен
	// (повторный клик по той же записи). Не ошибка данных, а «делать нечего».
	ErrRestoreNoop = errors.New("changelog: откат уже выполнен")
)

// Get возвращает запись истории по id.
func (s *Service) Get(ctx context.Context, id int64) (queries.ChangeLog, error) {
	row, err := s.store.GetChangeLogByID(ctx, id)
	if err != nil {
		if repo.IsNotFound(err) {
			return queries.ChangeLog{}, ErrLogNotFound
		}
		return queries.ChangeLog{}, fmt.Errorf("changelog: get %d: %w", id, err)
	}
	return row, nil
}

// LogRoleChangeTx фиксирует выдачу/снятие роли в истории — и пользователя
// (его лента ролей), и роли (кому выдавали). Undo работает с записью на
// стороне пользователя. userID/roleID — UUID-строки.
func (s *Service) LogRoleChangeTx(ctx context.Context, q *queries.Queries, action, userID, roleID, roleSlug, roleName string, createdBy uuid.UUID) error {
	payload := Fields{
		"role_id":   roleID,
		"role_slug": roleSlug,
		"role_name": roleName,
		"user_id":   userID,
	}
	var before, after Fields
	if action == ActionRoleRevoked {
		before = payload
	} else {
		after = payload
	}
	if err := s.write(ctx, q, EntityUser, userID, action, before, after, createdBy); err != nil {
		return err
	}
	return s.write(ctx, q, EntityRole, roleID, action, before, after, createdBy)
}

// LogRoleChangedTx фиксирует атомарную смену роли ОДНОЙ записью на стороне
// пользователя: before — прежняя роль, after — новая. Пустой fromSlug
// означает, что роли раньше не было (тогда before пустой).
func (s *Service) LogRoleChangedTx(ctx context.Context, q *queries.Queries, userID, fromSlug, fromName, toSlug, toName string, createdBy uuid.UUID) error {
	var before Fields
	if fromSlug != "" {
		before = Fields{"role_slug": fromSlug, "role_name": fromName}
	}
	after := Fields{"role_slug": toSlug, "role_name": toName}
	return s.write(ctx, q, EntityUser, userID, ActionRoleChanged, before, after, createdBy)
}

// RestoreFromLog откатывает сущность к состоянию before из записи лога.
// Поддержаны: профиль пользователя (updated) и выдача/снятие роли.
// Операция и запись о ней (restored_from_log) выполняются в одной транзакции.
func (s *Service) RestoreFromLog(ctx context.Context, logID int64, actorID uuid.UUID) error {
	if actorID == uuid.Nil {
		return fmt.Errorf("changelog: actor is required")
	}
	row, err := s.Get(ctx, logID)
	if err != nil {
		return err
	}
	before, err := decodeFields(row.Before)
	if err != nil {
		return fmt.Errorf("changelog: decode before: %w", err)
	}
	after, err := decodeFields(row.After)
	if err != nil {
		return fmt.Errorf("changelog: decode after: %w", err)
	}

	switch {
	case row.EntityType == EntityUser && row.Action == ActionUpdated:
		return s.restoreUserProfile(ctx, row.EntityID, before, actorID)
	case row.EntityType == EntityUser && row.Action == ActionRoleAssigned:
		// откат выдачи = снять роль (её данные в after)
		return s.restoreRole(ctx, row.EntityID, after, false, actorID)
	case row.EntityType == EntityUser && row.Action == ActionRoleRevoked:
		// откат снятия = снова выдать роль (её данные в before)
		return s.restoreRole(ctx, row.EntityID, before, true, actorID)
	case row.EntityType == EntityUser && row.Action == ActionRoleChanged:
		// откат смены = вернуть прежнюю роль (before) и снять новую (after)
		return s.restoreRoleChange(ctx, row.EntityID, before, after, actorID)
	default:
		return ErrRestoreUnsupported
	}
}

// restoreRoleChange откатывает смену роли: возвращает прежнюю роль (before)
// и снимает новую (after). Идемпотентно — если состояние уже целевое,
// соответствующий шаг пропускается. Пустой before означает, что роли не
// было: тогда откат только снимает новую роль.
func (s *Service) restoreRoleChange(ctx context.Context, userIDStr string, before, after Fields, actor uuid.UUID) error {
	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("changelog: bad user id %q: %w", userIDStr, err)
	}
	toRestore, _ := stringField(before, "role_slug") // прежняя роль — вернуть
	toRemove, _ := stringField(after, "role_slug")   // новая роль — снять

	return s.store.RunInTx(ctx, func(q *queries.Queries) error {
		changed := false
		if toRestore != "" {
			did, err := attachRoleBySlugIfMissing(ctx, q, uid, toRestore, actor)
			if err != nil {
				return err
			}
			changed = changed || did
		}
		if toRemove != "" && toRemove != toRestore {
			did, err := detachRoleBySlugIfPresent(ctx, q, uid, toRemove, actor)
			if err != nil {
				return err
			}
			changed = changed || did
		}
		// Ничего не поменялось (роль уже в целевом состоянии — повторный откат
		// той же записи) → не пишем пустой restored_from_log, отдаём noop.
		if !changed {
			return ErrRestoreNoop
		}
		// before/after меняем местами: текущее состояние «после смены» → «до».
		return s.write(ctx, q, EntityUser, userIDStr, ActionRestoredFromLog, after, before, actor)
	})
}

// attachRoleBySlugIfMissing выдаёт роль по slug, если её ещё нет. Возвращает
// true, если роль реально была добавлена (false — если уже была).
func attachRoleBySlugIfMissing(ctx context.Context, q *queries.Queries, uid uuid.UUID, slug string, actor uuid.UUID) (bool, error) {
	has, err := q.HasRole(ctx, queries.HasRoleParams{UserID: pgutil.PgUUID(uid), Lower: slug})
	if err != nil {
		return false, fmt.Errorf("check role: %w", err)
	}
	if has {
		return false, nil
	}
	role, err := q.GetRoleBySlug(ctx, slug)
	if err != nil {
		return false, fmt.Errorf("get role %q: %w", slug, err)
	}
	if _, err := q.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
		UserID: pgutil.PgUUID(uid), RoleID: role.ID, CreatedBy: pgutil.PgUUID(actor),
	}); err != nil {
		return false, err
	}
	return true, nil
}

// detachRoleBySlugIfPresent снимает роль по slug, если она активна. Возвращает
// true, если роль реально была снята (false — если её и не было).
func detachRoleBySlugIfPresent(ctx context.Context, q *queries.Queries, uid uuid.UUID, slug string, actor uuid.UUID) (bool, error) {
	has, err := q.HasRole(ctx, queries.HasRoleParams{UserID: pgutil.PgUUID(uid), Lower: slug})
	if err != nil {
		return false, fmt.Errorf("check role: %w", err)
	}
	if !has {
		return false, nil
	}
	role, err := q.GetRoleBySlug(ctx, slug)
	if err != nil {
		return false, fmt.Errorf("get role %q: %w", slug, err)
	}
	if err := q.DetachRoleFromUser(ctx, queries.DetachRoleFromUserParams{
		UserID: pgutil.PgUUID(uid), RoleID: role.ID, DeletedBy: pgutil.PgUUID(actor),
	}); err != nil {
		return false, err
	}
	return true, nil
}

// restoreUserProfile применяет поля before к профилю пользователя и пишет
// лог restored_from_log (before=текущее состояние, after=применённое).
func (s *Service) restoreUserProfile(ctx context.Context, userIDStr string, before Fields, actor uuid.UUID) error {
	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("changelog: bad user id %q: %w", userIDStr, err)
	}
	return s.store.RunInTx(ctx, func(q *queries.Queries) error {
		cur, err := q.GetUserByID(ctx, pgutil.PgUUID(uid))
		if err != nil {
			return fmt.Errorf("get user: %w", err)
		}
		// Профиль уже совпадает с before (повторный откат той же записи) →
		// не пишем пустой restored_from_log, отдаём noop.
		if !profileNeedsRestore(UserSnapshot(cur), before) {
			return ErrRestoreNoop
		}
		params := queries.UpdateUserProfileParams{ID: pgutil.PgUUID(uid)}
		if v, ok := stringField(before, "first_name"); ok {
			params.FirstName = &v
		}
		if v, ok := stringField(before, "last_name"); ok {
			params.LastName = &v
		}
		if v, ok := stringField(before, "middle_name"); ok {
			params.MiddleName = &v
		}
		if v, ok := stringField(before, "group_name"); ok {
			params.GroupName = &v
		}
		if v, ok := stringField(before, "birthday"); ok {
			if t, perr := time.Parse("2006-01-02", v); perr == nil {
				params.Birthday = pgtype.Date{Time: t, Valid: true}
			}
		}
		if _, err := q.UpdateUserProfile(ctx, params); err != nil {
			return fmt.Errorf("update profile: %w", err)
		}
		return s.write(ctx, q, EntityUser, userIDStr, ActionRestoredFromLog, UserSnapshot(cur), before, actor)
	})
}

// restoreRole выдаёт (assign=true) либо снимает (assign=false) роль и
// фиксирует факт отката записью restored_from_log на пользователе.
func (s *Service) restoreRole(ctx context.Context, userIDStr string, roleData Fields, assign bool, actor uuid.UUID) error {
	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("changelog: bad user id %q: %w", userIDStr, err)
	}
	roleIDStr, _ := stringField(roleData, "role_id")
	rid, err := uuid.Parse(roleIDStr)
	if err != nil {
		return fmt.Errorf("changelog: bad role id %q: %w", roleIDStr, err)
	}
	roleSlug, _ := stringField(roleData, "role_slug")
	return s.store.RunInTx(ctx, func(q *queries.Queries) error {
		// Откат роли — операция-переключатель. Если роль уже в целевом
		// состоянии (повторный откат той же записи) — это «делать нечего»:
		// сообщаем ErrRestoreNoop, а не падаем на конфликте уникального
		// индекса при повторном AttachRoleToUser.
		if roleSlug != "" {
			has, herr := q.HasRole(ctx, queries.HasRoleParams{UserID: pgutil.PgUUID(uid), Lower: roleSlug})
			if herr != nil {
				return fmt.Errorf("check role: %w", herr)
			}
			if assign == has { // выдать уже выданную ИЛИ снять уже снятую
				return ErrRestoreNoop
			}
		}
		if assign {
			if _, err := q.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
				UserID:    pgutil.PgUUID(uid),
				RoleID:    pgutil.PgUUID(rid),
				CreatedBy: pgutil.PgUUID(actor),
			}); err != nil {
				return fmt.Errorf("attach role: %w", err)
			}
		} else if err := q.DetachRoleFromUser(ctx, queries.DetachRoleFromUserParams{
			UserID:    pgutil.PgUUID(uid),
			RoleID:    pgutil.PgUUID(rid),
			DeletedBy: pgutil.PgUUID(actor),
		}); err != nil {
			return fmt.Errorf("detach role: %w", err)
		}

		var before, after Fields
		if assign {
			after = roleData
		} else {
			before = roleData
		}
		return s.write(ctx, q, EntityUser, userIDStr, ActionRestoredFromLog, before, after, actor)
	})
}

// profileNeedsRestore сообщает, изменит ли применение before текущее
// состояние профиля: true, если хотя бы одно поле из before отличается от
// текущего. Сравниваются только поля, присутствующие в before (откат трогает
// именно их). Используется для идемпотентности — чтобы повторный откат уже
// применённой записи не создавал пустой restored_from_log.
func profileNeedsRestore(current, before Fields) bool {
	for k, bv := range before {
		cv, ok := current[k]
		if !ok || cv != bv {
			return true
		}
	}
	return false
}

func stringField(f Fields, key string) (string, bool) {
	v, ok := f[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func decodeFields(b []byte) (Fields, error) {
	if len(b) == 0 {
		return Fields{}, nil
	}
	var f Fields
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f == nil {
		f = Fields{}
	}
	return f, nil
}
