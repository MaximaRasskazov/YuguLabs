package auth

// Управление профилем текущего пользователя:
//   - UpdateProfile — точечный апдейт ФИО/группы/даты рождения
//     (PATCH-семантика через COALESCE в sqlc-запросе UpdateUserProfile).
//   - ChangePassword — смена пароля с проверкой текущего, хешированием
//     нового и принудительным logout всех активных сессий пользователя.
//
// Эти методы не меняют email — для смены email нужен flow с
// подтверждением через старую почту, который пока не реализован.

import (
	"context"
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

// UpdateProfileInput — поля, которые можно поменять через PATCH /api/me.
// Все указатели опциональны: nil = «не трогать»; пустая строка валидна
// (например, очистить middle_name). Email сюда не входит — см. комментарий
// выше.
type UpdateProfileInput struct {
	FirstName  *string
	LastName   *string
	MiddleName *string
	GroupName  *string
	Birthday   *time.Time
}

// UpdateProfile применяет PATCH к профилю пользователя.
//
// Возвращает обновлённый профиль (тот же формат, что и Me).
// Если userID не существует — ErrUserNotFound.
//
// Валидация: first_name и last_name, если переданы, не могут быть пустой
// строкой после trim. Остальные поля могут быть пустыми (можно очистить
// middle_name или group_name).
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, in UpdateProfileInput) (*Profile, error) {
	if in.FirstName != nil {
		if strings.TrimSpace(*in.FirstName) == "" {
			return nil, fmt.Errorf("%w: first_name не может быть пустым", ErrInvalidEmail) //nolint:err113 // используем общий sentinel
		}
		v := strings.TrimSpace(*in.FirstName)
		in.FirstName = &v
	}
	if in.LastName != nil {
		if strings.TrimSpace(*in.LastName) == "" {
			return nil, fmt.Errorf("%w: last_name не может быть пустым", ErrInvalidEmail) //nolint:err113
		}
		v := strings.TrimSpace(*in.LastName)
		in.LastName = &v
	}

	params := queries.UpdateUserProfileParams{
		ID:         pgutil.PgUUID(userID),
		FirstName:  in.FirstName,
		LastName:   in.LastName,
		MiddleName: in.MiddleName,
		GroupName:  in.GroupName,
	}
	if in.Birthday != nil {
		params.Birthday = pgtype.Date{Time: *in.Birthday, Valid: true}
	}

	// Апдейт + запись в change_logs одной транзакцией: сначала читаем
	// текущее состояние (before), затем обновляем (after) и логируем —
	// чтобы история и профиль не разъехались при сбое.
	err := s.store.RunInTx(ctx, func(q *queries.Queries) error {
		before, err := q.GetUserByID(ctx, pgutil.PgUUID(userID))
		if err != nil {
			if repo.IsNotFound(err) {
				return ErrUserNotFound
			}
			return fmt.Errorf("get user: %w", err)
		}
		updated, err := q.UpdateUserProfile(ctx, params)
		if err != nil {
			return fmt.Errorf("update profile: %w", err)
		}
		if s.changelog != nil {
			if err := s.changelog.LogUpdatedTx(ctx, q, changelog.EntityUser, userID.String(),
				changelog.UserSnapshot(before), changelog.UserSnapshot(updated), userID); err != nil {
				return fmt.Errorf("changelog updated: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Возвращаем свежий Profile целиком — фронт сразу обновляет UI без
	// лишнего round-trip на /me.
	return s.Me(ctx, userID)
}

// ChangePassword меняет пароль пользователя:
//
//  1. Проверяет, что current_password совпадает с хешем в БД.
//  2. Хеширует новый пароль (bcrypt).
//  3. Атомарно: обновляет password_hash + ревокует все access-токены
//     пользователя (RevokeAllAccessTokensForUser). Это нужно чтобы
//     злоумышленник с украденным access не остался залогиненным после
//     смены пароля.
//
// Refresh-токены тоже становятся непригодны при следующем рефреше:
// в БД они привязаны к user_id, а access после revoke + 401 →
// interceptor попробует refresh, и token.Service увидит, что вся
// серия была отозвана. Дополнительный таблично-уровневый revoke
// refresh-токенов можно добавить, если ТЗ потребует instant-kill.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, current, next string) error {
	if len(next) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if current == next {
		return ErrSamePassword
	}

	user, err := s.store.GetUserByID(ctx, pgutil.PgUUID(userID))
	if err != nil {
		if repo.IsNotFound(err) {
			return ErrUserNotFound
		}
		return fmt.Errorf("get user: %w", err)
	}

	if err := checkPassword(user.PasswordHash, current); err != nil {
		// Не отличаем "не найден" от "неверный пароль" — но здесь
		// пользователь уже найден, так что эта ветка значит именно
		// несовпадение хеша.
		return ErrInvalidPassword
	}

	hash, err := hashPassword(next)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	return s.store.RunInTx(ctx, func(q *queries.Queries) error {
		if err := q.UpdateUserPassword(ctx, queries.UpdateUserPasswordParams{
			ID:           user.ID,
			PasswordHash: hash,
		}); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		// Гасим все активные access-токены — пользователь должен
		// перелогиниться явно (или фронт получит 401 и сделает /refresh,
		// который тоже упадёт по логике replay-detection).
		if err := q.RevokeAllAccessTokensForUser(ctx, user.ID); err != nil {
			return fmt.Errorf("revoke tokens: %w", err)
		}
		if s.changelog != nil {
			// Фиксируем сам факт смены пароля (без хеша) в истории.
			if err := s.changelog.LogCustomTx(ctx, q, changelog.EntityUser, userID.String(),
				changelog.ActionPasswordChanged, nil, nil, userID); err != nil {
				return fmt.Errorf("changelog password: %w", err)
			}
		}
		return nil
	})
}
