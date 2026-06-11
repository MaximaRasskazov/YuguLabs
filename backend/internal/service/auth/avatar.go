package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
)

// MaxAvatarBytes — максимальный размер файла аватара (2 МиБ).
const MaxAvatarBytes = 2 << 20

// allowedAvatarTypes — MIME-типы, которые принимаем. Тип определяется по
// сигнатуре содержимого (http.DetectContentType), а не из заголовка
// клиента — так нельзя подсунуть исполняемый файл под видом картинки.
var allowedAvatarTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// Ошибки flow аватара.
var (
	ErrAvatarEmpty    = errors.New("auth: пустой файл аватара")
	ErrAvatarTooLarge = errors.New("auth: файл аватара слишком большой")
	ErrAvatarType     = errors.New("auth: недопустимый тип файла (нужен JPEG, PNG или WebP)")
	ErrAvatarNotFound = errors.New("auth: аватар не найден")
)

// Avatar — содержимое аватара для отдачи клиенту.
type Avatar struct {
	Content     []byte
	ContentType string
	UpdatedAt   time.Time
}

// SetAvatar валидирует и сохраняет аватар пользователя. Тип определяется
// по сигнатуре файла; размер ограничен MaxAvatarBytes.
func (s *Service) SetAvatar(ctx context.Context, userID uuid.UUID, content []byte) error {
	if len(content) == 0 {
		return ErrAvatarEmpty
	}
	if len(content) > MaxAvatarBytes {
		return ErrAvatarTooLarge
	}
	ct := http.DetectContentType(content)
	if !allowedAvatarTypes[ct] {
		return ErrAvatarType
	}
	return s.store.UpsertUserAvatar(ctx, queries.UpsertUserAvatarParams{
		UserID:      pgutil.PgUUID(userID),
		Content:     content,
		ContentType: ct,
	})
}

// GetAvatar возвращает байты аватара. ErrAvatarNotFound, если его нет.
func (s *Service) GetAvatar(ctx context.Context, userID uuid.UUID) (*Avatar, error) {
	row, err := s.store.GetUserAvatar(ctx, pgutil.PgUUID(userID))
	if err != nil {
		if repo.IsNotFound(err) {
			return nil, ErrAvatarNotFound
		}
		return nil, fmt.Errorf("get avatar: %w", err)
	}
	return &Avatar{
		Content:     row.Content,
		ContentType: row.ContentType,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

// DeleteAvatar удаляет аватар. Идемпотентно: отсутствие аватара — не ошибка.
func (s *Service) DeleteAvatar(ctx context.Context, userID uuid.UUID) error {
	if err := s.store.DeleteUserAvatar(ctx, pgutil.PgUUID(userID)); err != nil {
		return fmt.Errorf("delete avatar: %w", err)
	}
	return nil
}

// avatarUpdatedAt возвращает время последнего обновления аватара (версия
// для cache-busting в URL) либо nil, если аватара нет. Не выгружает байты.
func (s *Service) avatarUpdatedAt(ctx context.Context, userID pgtype.UUID) *time.Time {
	meta, err := s.store.GetUserAvatarMeta(ctx, userID)
	if err != nil {
		return nil
	}
	t := meta.UpdatedAt.Time
	return &t
}
