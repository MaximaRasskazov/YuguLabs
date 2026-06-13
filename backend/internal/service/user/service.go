// Package user реализует выборки списков пользователей с фильтрами и
// пагинацией. Не дублирует функционал auth-сервиса (Register/Login/Me) —
// это отдельный admin-style API для деканата:
//
//   - GET /api/users?role=teacher&search=иванов&limit=20 → выбрать
//     преподавателя из выпадающего списка в форме создания пересдачи
//   - GET /api/users?role=student&group_name=ИВТ-21 → ростер группы
//   - админ может смотреть всех без фильтров
//
// Защищается permission'ом users.view (есть у dean и admin из сидов).
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
)

// Ошибки админского редактирования профиля.
var (
	// ErrUserNotFound — целевой пользователь не найден.
	ErrUserNotFound = errors.New("user: пользователь не найден")
	// ErrInvalidProfile — некорректные данные (например, пустые ФИО).
	ErrInvalidProfile = errors.New("user: некорректные данные профиля")
)

// Service — выборки и админское редактирование пользователей.
type Service struct {
	store     *repo.Store
	changelog *changelog.Service // может быть nil — тогда правки не пишутся в change_logs
}

func New(store *repo.Store) *Service {
	return &Service{store: store}
}

// SetChangelog включает запись админских правок профиля в change_logs.
func (s *Service) SetChangelog(c *changelog.Service) { s.changelog = c }

// UpdateInput — поля профиля, доступные администратору для правки.
// nil = «не трогать»; пустая строка очищает (middle_name / group_name).
// Email не входит — его смена требует подтверждения через почту.
type UpdateInput struct {
	FirstName  *string
	LastName   *string
	MiddleName *string
	GroupName  *string
	Birthday   *time.Time
}

// UpdateUser применяет правку профиля целевого пользователя администратором.
//
// Изменение пишется в change_logs с created_by = actorID (администратор),
// entity_id = targetID — поэтому в истории пользователя видно, что правку
// сделал админ, и её можно откатить. Всё в одной транзакции: если лог не
// записался, правка откатывается.
func (s *Service) UpdateUser(ctx context.Context, targetID uuid.UUID, in UpdateInput, actorID uuid.UUID) (queries.User, error) {
	if in.FirstName != nil {
		v := strings.TrimSpace(*in.FirstName)
		if v == "" {
			return queries.User{}, fmt.Errorf("%w: first_name не может быть пустым", ErrInvalidProfile)
		}
		in.FirstName = &v
	}
	if in.LastName != nil {
		v := strings.TrimSpace(*in.LastName)
		if v == "" {
			return queries.User{}, fmt.Errorf("%w: last_name не может быть пустым", ErrInvalidProfile)
		}
		in.LastName = &v
	}

	params := queries.UpdateUserProfileParams{
		ID:         pgutil.PgUUID(targetID),
		FirstName:  in.FirstName,
		LastName:   in.LastName,
		MiddleName: in.MiddleName,
		GroupName:  in.GroupName,
	}
	if in.Birthday != nil {
		params.Birthday = pgtype.Date{Time: *in.Birthday, Valid: true}
	}

	var updated queries.User
	err := s.store.RunInTx(ctx, func(q *queries.Queries) error {
		before, err := q.GetUserByID(ctx, pgutil.PgUUID(targetID))
		if err != nil {
			if repo.IsNotFound(err) {
				return ErrUserNotFound
			}
			return fmt.Errorf("get user: %w", err)
		}
		updated, err = q.UpdateUserProfile(ctx, params)
		if err != nil {
			return fmt.Errorf("update user profile: %w", err)
		}
		if s.changelog != nil {
			if err := s.changelog.LogUpdatedTx(ctx, q, changelog.EntityUser, targetID.String(),
				changelog.UserSnapshot(before), changelog.UserSnapshot(updated), actorID); err != nil {
				return fmt.Errorf("changelog updated: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return queries.User{}, err
	}
	return updated, nil
}

// ListInput — фильтры выборки. Все поля опциональны.
type ListInput struct {
	RoleSlug  string // 'student' / 'teacher' / 'dean' / 'admin' / ''
	Search    string // подстрока для ILIKE по email/first_name/last_name
	GroupName string // точный матч для студентов
	Limit     int32
	Offset    int32
}

// UserWithRoles — пользователь с заранее подтянутыми ролями.
// Сделано отдельным типом, чтобы handler не делал N+1 запросов
// "для каждого user'а получи его роли" — мы это делаем здесь
// одной пачкой.
type UserWithRoles struct {
	User  queries.User
	Roles []queries.Role
}

// ListResult — пагинированный ответ.
type ListResult struct {
	Items  []UserWithRoles
	Total  int64
	Limit  int32
	Offset int32
}

const (
	defaultLimit = int32(50)
	maxLimit     = int32(200)
)

// List возвращает список пользователей с применёнными фильтрами и
// прикреплёнными ролями каждого. Для производительности роли тянем
// одним запросом на пользователя (ListRolesForUser): обычно списки
// небольшие, до 200 строк, и оптимизировать через JOIN агрегацию
// преждевременно — текущая нагрузка курсача это не требует.
func (s *Service) List(ctx context.Context, in ListInput) (*ListResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := in.Offset
	if offset < 0 {
		offset = 0
	}

	params := queries.ListUsersParams{
		LimitN:  limit,
		OffsetN: offset,
	}
	countParams := queries.CountUsersParams{}

	// sqlc ожидает *string для опциональных полей (emit_pointers_for_null_types).
	if v := strings.TrimSpace(in.RoleSlug); v != "" {
		params.RoleSlug = &v
		countParams.RoleSlug = &v
	}
	if v := strings.TrimSpace(in.Search); v != "" {
		params.Search = &v
		countParams.Search = &v
	}
	if v := strings.TrimSpace(in.GroupName); v != "" {
		params.GroupName = &v
		countParams.GroupName = &v
	}

	rows, err := s.store.ListUsers(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	total, err := s.store.CountUsers(ctx, countParams)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}

	items := make([]UserWithRoles, 0, len(rows))
	for _, u := range rows {
		uid := uuid.UUID(u.ID.Bytes)
		roles, err := s.store.ListRolesForUser(ctx, pgutil.PgUUID(uid))
		if err != nil {
			return nil, fmt.Errorf("list roles for %s: %w", uid, err)
		}
		items = append(items, UserWithRoles{User: u, Roles: roles})
	}

	return &ListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}
