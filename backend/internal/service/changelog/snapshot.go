package changelog

import (
	"reflect"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
)

// Entity types, история которых доступна в админ-разделе (лаба про
// логирование мутаций users/roles/permissions). Доменные сущности
// (debt/retake/...) логируются своими сервисами под своими строками.
const (
	EntityUser       = "user"
	EntityRole       = "role"
	EntityPermission = "permission"
)

// FieldChange — одно изменившееся поле для ответа истории (changed_fields).
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

// Diff вычисляет изменившиеся поля между before и after. В результат
// попадают только реально различающиеся ключи: { поле: {old, new} }.
// Нужен для story-эндпоинтов — в БД храним полный срез, наружу отдаём дифф.
func Diff(before, after Fields) map[string]FieldChange {
	changed := make(map[string]FieldChange)
	for k, bv := range before {
		av, ok := after[k]
		if !ok || !reflect.DeepEqual(bv, av) {
			changed[k] = FieldChange{Old: bv, New: av} // av == nil, если ключа нет в after
		}
	}
	for k, av := range after {
		if _, ok := before[k]; ok {
			continue // уже рассмотрели выше
		}
		changed[k] = FieldChange{Old: nil, New: av}
	}
	return changed
}

// ChangedFields декодирует before/after записи лога и возвращает только
// изменившиеся поля — готовый changed_fields для story-эндпоинтов.
func ChangedFields(row queries.ChangeLog) (map[string]FieldChange, error) {
	return ChangedFieldsRaw(row.Before, row.After)
}

// ChangedFieldsRaw — то же, но из сырых JSONB-байтов (для join-выборок,
// где нет цельной queries.ChangeLog).
func ChangedFieldsRaw(before, after []byte) (map[string]FieldChange, error) {
	b, err := decodeFields(before)
	if err != nil {
		return nil, err
	}
	a, err := decodeFields(after)
	if err != nil {
		return nil, err
	}
	return Diff(b, a), nil
}

// UserSnapshot формирует JSON-срез пользователя для change_logs.
// Намеренно НЕ включает password_hash и прочие секреты — в лог уходят
// только отображаемые поля профиля.
func UserSnapshot(u queries.User) Fields {
	f := Fields{
		"email":      u.Email,
		"first_name": u.FirstName,
		"last_name":  u.LastName,
	}
	if u.MiddleName != nil {
		f["middle_name"] = *u.MiddleName
	}
	if u.GroupName != nil {
		f["group_name"] = *u.GroupName
	}
	if u.Birthday.Valid {
		f["birthday"] = u.Birthday.Time.Format("2006-01-02")
	}
	return f
}
