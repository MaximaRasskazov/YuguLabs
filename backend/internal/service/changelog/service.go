// Package changelog реализует запись срезов сущностей до и после
// мутаций в таблицу change_logs.
//
// Назначение и отличие от audit_log:
//   - audit_log фиксирует ЧТО произошло (action), кто, когда — для
//     событий безопасности.
//   - change_logs фиксирует ПОЛНЫЕ ПОЛЯ сущности до и после мутации
//     в JSONB. Это даёт читаемую историю изменений в UI ("23 мая
//     преподаватель X сменил оценку 2 → 4") и техническую возможность
//     отката (восстановить before в строку сущности).
//
// Все методы принимают entityType (строка типа "debt", "retake",
// "role"), entityID (UUID в виде строки, varchar(255) в БД) и
// createdBy — автора мутации.
//
// before/after передаются как map[string]any: caller сам формирует
// JSON-friendly срез. Это спасает от случайного попадания в лог
// password_hash, токенов и других секретов — каждый сервис явно
// перечисляет, какие поля документирует.
package changelog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
)

// Action — слаги операций в change_logs. Соответствуют комментарию
// в миграции 00009_create_change_logs.sql.
const (
	ActionCreated         = "created"
	ActionUpdated         = "updated"
	ActionSoftDeleted     = "soft_deleted"
	ActionHardDeleted     = "hard_deleted"
	ActionRestored        = "restored"
	ActionRestoredFromLog = "restored_from_log"
	// ActionPasswordChanged — смена пароля. before/after пустые: сам пароль
	// (хеш) в историю принципиально не пишется, фиксируем только факт.
	ActionPasswordChanged = "password_changed"
)

type Fields = map[string]any

// Service пишет change_logs-записи. Концептуально stateless.
type Service struct {
	store *repo.Store
}

func New(store *repo.Store) *Service {
	return &Service{store: store}
}

// LogCreatedTx — before={}, after=fields. Вызывается в той же
// транзакции, что и сам INSERT новой сущности.
func (s *Service) LogCreatedTx(ctx context.Context, q *queries.Queries, entityType, entityID string, after Fields, createdBy uuid.UUID) error {
	return s.write(ctx, q, entityType, entityID, ActionCreated, nil, after, createdBy)
}

// LogUpdatedTx — before=прежние_поля, after=новые. Обычно сервис
// сначала читает текущее состояние, делает UPDATE, формирует after
// и пишет лог в одной транзакции.
func (s *Service) LogUpdatedTx(ctx context.Context, q *queries.Queries, entityType, entityID string, before, after Fields, createdBy uuid.UUID) error {
	return s.write(ctx, q, entityType, entityID, ActionUpdated, before, after, createdBy)
}

// LogSoftDeletedTx — before=поля_сущности, after={}.
func (s *Service) LogSoftDeletedTx(ctx context.Context, q *queries.Queries, entityType, entityID string, before Fields, createdBy uuid.UUID) error {
	return s.write(ctx, q, entityType, entityID, ActionSoftDeleted, before, nil, createdBy)
}

// LogRestoredTx — before=поля_на_момент_удаления, after=восстановленные.
func (s *Service) LogRestoredTx(ctx context.Context, q *queries.Queries, entityType, entityID string, before, after Fields, createdBy uuid.UUID) error {
	return s.write(ctx, q, entityType, entityID, ActionRestored, before, after, createdBy)
}

// LogCustomTx — escape hatch для нестандартных action'ов (например,
// restored_from_log при undo). Caller сам отвечает за корректность
// семантики before/after.
func (s *Service) LogCustomTx(ctx context.Context, q *queries.Queries, entityType, entityID, action string, before, after Fields, createdBy uuid.UUID) error {
	return s.write(ctx, q, entityType, entityID, action, before, after, createdBy)
}

// ListForEntity — история изменений конкретной сущности (новые сверху).
func (s *Service) ListForEntity(ctx context.Context, entityType, entityID string, limit, offset int32) ([]queries.ChangeLog, error) {
	return s.store.ListChangeLogsForEntity(ctx, queries.ListChangeLogsForEntityParams{
		EntityType: entityType,
		EntityID:   entityID,
		Limit:      limit,
		Offset:     offset,
	})
}

// ListRecent — общий журнал последних изменений (users/roles/permissions)
// с подтянутым ФИО для user-записей. Для админ-вкладки «Журнал изменений».
func (s *Service) ListRecent(ctx context.Context, limit, offset int32) ([]queries.ListRecentChangeLogsRow, error) {
	return s.store.ListRecentChangeLogs(ctx, queries.ListRecentChangeLogsParams{
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Service) write(ctx context.Context, q *queries.Queries, entityType, entityID, action string, before, after Fields, createdBy uuid.UUID) error {
	if entityType == "" {
		return fmt.Errorf("changelog: entity_type is required")
	}
	if entityID == "" {
		return fmt.Errorf("changelog: entity_id is required")
	}
	if action == "" {
		return fmt.Errorf("changelog: action is required")
	}
	if createdBy == uuid.Nil {
		return fmt.Errorf("changelog: created_by is required")
	}

	beforeJSON, err := encodeFields(before)
	if err != nil {
		return fmt.Errorf("changelog: marshal before: %w", err)
	}
	afterJSON, err := encodeFields(after)
	if err != nil {
		return fmt.Errorf("changelog: marshal after: %w", err)
	}

	if _, err := q.CreateChangeLog(ctx, queries.CreateChangeLogParams{
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Before:     beforeJSON,
		After:      afterJSON,
		CreatedBy:  pgutil.PgUUID(createdBy),
	}); err != nil {
		return fmt.Errorf("changelog: insert: %w", err)
	}
	return nil
}

// encodeFields сериализует map в JSON. nil → пустой объект {}, что
// соответствует CHECK-constraint в миграции (DEFAULT '{}'::jsonb).
func encodeFields(f Fields) ([]byte, error) {
	if f == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(f)
}
