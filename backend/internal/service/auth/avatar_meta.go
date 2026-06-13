package auth

// Версия аватара для профиля. Сами операции с фотографиями вынесены в
// пакет service/photo (загрузка, миниатюра, скачивание оригинала, архив);
// auth-сервису для сборки /me нужно лишь время последнего обновления —
// оно идёт в ?v= URL аватара для cache-busting.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// avatarUpdatedAt возвращает время последнего обновления активной
// фотографии пользователя либо nil, если её нет. Не выгружает байты.
func (s *Service) avatarUpdatedAt(ctx context.Context, userID pgtype.UUID) *time.Time {
	ts, err := s.store.GetActiveUserPhotoVersion(ctx, userID)
	if err != nil || !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}
