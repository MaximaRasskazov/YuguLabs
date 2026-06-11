package dto

import (
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
)

// ChangeLogEntry — запись истории изменений для story-эндпоинтов.
// changed_fields содержит ТОЛЬКО изменившиеся поля ({поле: {old,new}}),
// хотя в БД хранится полный срез before/after.
type ChangeLogEntry struct {
	ID            int64                            `json:"id"`
	EntityType    string                           `json:"entity_type"`
	EntityID      string                           `json:"entity_id"`
	Action        string                           `json:"action"`
	ChangedFields map[string]changelog.FieldChange `json:"changed_fields"`
	CreatedAt     time.Time                        `json:"created_at"`
	CreatedBy     uuid.UUID                        `json:"created_by"`
}

// FromChangeLog маппит sqlc-запись в DTO, вычисляя дифф before/after.
func FromChangeLog(row queries.ChangeLog) (ChangeLogEntry, error) {
	cf, err := changelog.ChangedFields(row)
	if err != nil {
		return ChangeLogEntry{}, err
	}
	return ChangeLogEntry{
		ID:            row.ID,
		EntityType:    row.EntityType,
		EntityID:      row.EntityID,
		Action:        row.Action,
		ChangedFields: cf,
		CreatedAt:     row.CreatedAt.Time,
		CreatedBy:     pgutil.UUID(row.CreatedBy),
	}, nil
}

// FromChangeLogs маппит срез записей истории.
func FromChangeLogs(rows []queries.ChangeLog) ([]ChangeLogEntry, error) {
	out := make([]ChangeLogEntry, 0, len(rows))
	for _, r := range rows {
		e, err := FromChangeLog(r)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// JournalEntry — запись общего журнала. В отличие от story добавляет
// человекочитаемое поле subject (ФИО затронутого пользователя).
type JournalEntry struct {
	ID            int64                            `json:"id"`
	EntityType    string                           `json:"entity_type"`
	EntityID      string                           `json:"entity_id"`
	Action        string                           `json:"action"`
	Subject       string                           `json:"subject"`
	ChangedFields map[string]changelog.FieldChange `json:"changed_fields"`
	CreatedAt     time.Time                        `json:"created_at"`
}

// FromRecentChangeLogs маппит строки журнала (с подтянутым ФИО) в DTO.
func FromRecentChangeLogs(rows []queries.ListRecentChangeLogsRow) ([]JournalEntry, error) {
	out := make([]JournalEntry, 0, len(rows))
	for _, r := range rows {
		cf, err := changelog.ChangedFieldsRaw(r.Before, r.After)
		if err != nil {
			return nil, err
		}
		out = append(out, JournalEntry{
			ID:            r.ID,
			EntityType:    r.EntityType,
			EntityID:      r.EntityID,
			Action:        r.Action,
			Subject:       joinName(r.SubjectLastName, r.SubjectFirstName, r.SubjectMiddleName),
			ChangedFields: cf,
			CreatedAt:     r.CreatedAt.Time,
		})
	}
	return out, nil
}

// joinName собирает «Фамилия Имя Отчество» из nullable-полей JOIN'а.
// Пустая строка, если ФИО нет (не-user сущность).
func joinName(last, first, middle *string) string {
	parts := make([]string, 0, 3)
	for _, p := range []*string{last, first, middle} {
		if p != nil && *p != "" {
			parts = append(parts, *p)
		}
	}
	return strings.Join(parts, " ")
}
