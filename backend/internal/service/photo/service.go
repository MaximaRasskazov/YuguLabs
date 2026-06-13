// Package photo реализует загрузку, хранение и выдачу фотографий профиля.
//
// Из одного загруженного снимка сервис формирует две производные
// (см. internal/imageutil):
//   - сжатую копию оригинала — отдаётся только владельцу через защищённый
//     маршрут скачивания (прямого URL у файла в БД нет);
//   - квадратный аватар 128×128 — публичная миниатюра для <img>.
//
// Контроллеры остаются тонкими: вся валидация (анти-спуфинг, лимиты,
// генерация производных) и работа с БД — здесь. Архивная выгрузка для
// администратора вынесена в archive.go.
package photo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/imageutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
)

// MaxUploadBytes — потолок размера ВХОДНОГО файла (16 МиБ). Сделан с
// запасом относительно итоговой сжатой копии: смысл в том, чтобы принять
// «тяжёлый» снимок (4K с телефона) и ужать его на сервере, а не заставлять
// клиента подгонять размер. Реальное хранимое содержимое после сжатия —
// в разы меньше.
const MaxUploadBytes = 16 << 20

// Слаги для записи действий с фото в change_logs. Отдельный entity_type,
// чтобы не засорять историю профиля пользователя (story-эндпоинты).
const (
	entityUserPhoto     = "user_photo"
	actionPhotoUploaded = "photo_uploaded"
	actionPhotoDeleted  = "photo_deleted"
)

// Ошибки flow фотографии. Хендлер мапит их в HTTP-статусы.
var (
	// ErrEmpty — пустой файл.
	ErrEmpty = errors.New("photo: пустой файл")
	// ErrTooLarge — входной файл превышает MaxUploadBytes.
	ErrTooLarge = errors.New("photo: файл слишком большой")
	// ErrInvalidImage — файл не является изображением (в т.ч. подмена
	// расширения: внутри не картинка).
	ErrInvalidImage = errors.New("photo: файл не является изображением (JPEG, PNG или WebP)")
	// ErrImageTooLarge — разрешение изображения превышает допустимое
	// (защита от decompression bomb).
	ErrImageTooLarge = errors.New("photo: слишком большое разрешение изображения")
	// ErrNotFound — у пользователя нет активной фотографии.
	ErrNotFound = errors.New("photo: фотография не найдена")
)

// Service инкапсулирует операции с фотографиями профиля.
type Service struct {
	store     *repo.Store
	changelog *changelog.Service // может быть nil — тогда действия с фото не пишутся в change_logs
}

// New собирает Service. Без I/O.
func New(store *repo.Store) *Service {
	return &Service{store: store}
}

// SetChangelog включает логирование действий с фото (upload/delete) в
// change_logs. Опционально — вызывается из main при сборке зависимостей.
func (s *Service) SetChangelog(c *changelog.Service) { s.changelog = c }

// Info — метаданные активной фотографии (без бинарного содержимого).
type Info struct {
	ID                  int64
	UserID              uuid.UUID
	OriginalName        string
	Description         *string
	Format              string
	SizeBytes           int32
	Width               int32
	Height              int32
	OriginalContentType string
	AvatarContentType   string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Avatar — байты квадратной миниатюры для публичной отдачи.
type Avatar struct {
	Content     []byte
	ContentType string
	UpdatedAt   time.Time
}

// Original — байты сжатого оригинала для защищённого скачивания.
type Original struct {
	Content      []byte
	ContentType  string
	OriginalName string
	Format       string
}

// Upload валидирует снимок, формирует производные и сохраняет их как новую
// активную фотографию пользователя. Прежняя активная мягко удаляется в той
// же транзакции — на пользователя остаётся ровно одна актуальная фотография,
// а история сохраняется (deleted_at). actorID — кто инициировал загрузку
// (обычно сам владелец) — пишется в change_logs.
func (s *Service) Upload(ctx context.Context, userID uuid.UUID, raw []byte, originalName, description string, actorID uuid.UUID) (*Info, error) {
	if len(raw) == 0 {
		return nil, ErrEmpty
	}
	if len(raw) > MaxUploadBytes {
		return nil, ErrTooLarge
	}

	processed, err := imageutil.Process(raw)
	if err != nil {
		return nil, mapImageError(err)
	}

	name := sanitizeName(originalName)
	var desc *string
	if d := strings.TrimSpace(description); d != "" {
		desc = &d
	}

	var info *Info
	err = s.store.RunInTx(ctx, func(q *queries.Queries) error {
		if err := q.SoftDeleteActiveUserPhoto(ctx, pgutil.PgUUID(userID)); err != nil {
			return fmt.Errorf("photo: soft-delete previous: %w", err)
		}
		row, err := q.InsertUserPhoto(ctx, queries.InsertUserPhotoParams{
			UserID:              pgutil.PgUUID(userID),
			OriginalName:        name,
			Description:         desc,
			Format:              processed.Format,
			SizeBytes:           int32(len(processed.Original)),
			Width:               int32(processed.Width),
			Height:              int32(processed.Height),
			OriginalContent:     processed.Original,
			OriginalContentType: processed.OriginalContentType,
			AvatarContent:       processed.Avatar,
			AvatarContentType:   processed.AvatarContentType,
		})
		if err != nil {
			return fmt.Errorf("photo: insert: %w", err)
		}
		info = &Info{
			ID:                  row.ID,
			UserID:              pgutil.UUID(row.UserID),
			OriginalName:        row.OriginalName,
			Description:         row.Description,
			Format:              row.Format,
			SizeBytes:           row.SizeBytes,
			Width:               row.Width,
			Height:              row.Height,
			OriginalContentType: row.OriginalContentType,
			AvatarContentType:   row.AvatarContentType,
			CreatedAt:           row.CreatedAt.Time,
			UpdatedAt:           row.UpdatedAt.Time,
		}
		if s.changelog != nil {
			after := changelog.Fields{
				"original_name": row.OriginalName,
				"format":        row.Format,
				"size_bytes":    row.SizeBytes,
				"width":         row.Width,
				"height":        row.Height,
			}
			if err := s.changelog.LogCustomTx(ctx, q, entityUserPhoto, userID.String(), actionPhotoUploaded, nil, after, actorID); err != nil {
				return fmt.Errorf("photo: changelog upload: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return info, nil
}

// Info возвращает метаданные активной фотографии пользователя.
// ErrNotFound, если её нет.
func (s *Service) Info(ctx context.Context, userID uuid.UUID) (*Info, error) {
	row, err := s.store.GetActiveUserPhotoMeta(ctx, pgutil.PgUUID(userID))
	if err != nil {
		if repo.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("photo: get meta: %w", err)
	}
	return &Info{
		ID:                  row.ID,
		UserID:              pgutil.UUID(row.UserID),
		OriginalName:        row.OriginalName,
		Description:         row.Description,
		Format:              row.Format,
		SizeBytes:           row.SizeBytes,
		Width:               row.Width,
		Height:              row.Height,
		OriginalContentType: row.OriginalContentType,
		AvatarContentType:   row.AvatarContentType,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}, nil
}

// Avatar возвращает байты квадратной миниатюры. ErrNotFound, если фото нет.
func (s *Service) Avatar(ctx context.Context, userID uuid.UUID) (*Avatar, error) {
	row, err := s.store.GetActiveUserAvatar(ctx, pgutil.PgUUID(userID))
	if err != nil {
		if repo.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("photo: get avatar: %w", err)
	}
	return &Avatar{
		Content:     row.AvatarContent,
		ContentType: row.AvatarContentType,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

// Original возвращает байты сжатого оригинала для скачивания владельцем.
// ErrNotFound, если фото нет.
func (s *Service) Original(ctx context.Context, userID uuid.UUID) (*Original, error) {
	row, err := s.store.GetActiveUserOriginal(ctx, pgutil.PgUUID(userID))
	if err != nil {
		if repo.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("photo: get original: %w", err)
	}
	return &Original{
		Content:      row.OriginalContent,
		ContentType:  row.OriginalContentType,
		OriginalName: row.OriginalName,
		Format:       row.Format,
	}, nil
}

// Delete мягко удаляет активную фотографию пользователя. Идемпотентно:
// если активной фотографии нет — это не ошибка (повторный DELETE безопасен).
// Фактическое удаление пишется в change_logs.
func (s *Service) Delete(ctx context.Context, userID, actorID uuid.UUID) error {
	return s.store.RunInTx(ctx, func(q *queries.Queries) error {
		meta, err := q.GetActiveUserPhotoMeta(ctx, pgutil.PgUUID(userID))
		if err != nil {
			if repo.IsNotFound(err) {
				return nil // нечего удалять — идемпотентно
			}
			return fmt.Errorf("photo: get meta: %w", err)
		}
		if err := q.SoftDeleteActiveUserPhoto(ctx, pgutil.PgUUID(userID)); err != nil {
			return fmt.Errorf("photo: soft-delete: %w", err)
		}
		if s.changelog != nil {
			before := changelog.Fields{
				"original_name": meta.OriginalName,
				"format":        meta.Format,
				"size_bytes":    meta.SizeBytes,
			}
			if err := s.changelog.LogCustomTx(ctx, q, entityUserPhoto, userID.String(), actionPhotoDeleted, before, nil, actorID); err != nil {
				return fmt.Errorf("photo: changelog delete: %w", err)
			}
		}
		return nil
	})
}

// AdminPhoto — метаданные одной фотографии с данными владельца (для
// админского списка «все фото»). Без бинарного содержимого.
type AdminPhoto struct {
	ID           int64
	UserID       uuid.UUID
	Email        string
	FullName     string
	GroupName    string
	OriginalName string
	Description  *string
	Format       string
	SizeBytes    int32
	Width        int32
	Height       int32
	CreatedAt    time.Time
}

// ListAll возвращает метаданные всех активных фотографий с данными
// владельцев — для администратора (просмотр всех фото). Байты не выгружаются:
// миниатюру админ видит по avatar_url, оригинал — в архиве.
func (s *Service) ListAll(ctx context.Context) ([]AdminPhoto, error) {
	rows, err := s.store.ListAllActiveUserPhotosMeta(ctx)
	if err != nil {
		return nil, fmt.Errorf("photo: list all: %w", err)
	}
	out := make([]AdminPhoto, 0, len(rows))
	for _, r := range rows {
		out = append(out, AdminPhoto{
			ID:           r.ID,
			UserID:       pgutil.UUID(r.UserID),
			Email:        r.Email,
			FullName:     strings.Join(nonEmpty(r.LastName, r.FirstName, deref(r.MiddleName)), " "),
			GroupName:    deref(r.GroupName),
			OriginalName: r.OriginalName,
			Description:  r.Description,
			Format:       r.Format,
			SizeBytes:    r.SizeBytes,
			Width:        r.Width,
			Height:       r.Height,
			CreatedAt:    r.CreatedAt.Time,
		})
	}
	return out, nil
}

// mapImageError переводит ошибки imageutil в доменные ошибки photo.
func mapImageError(err error) error {
	switch {
	case errors.Is(err, imageutil.ErrEmpty):
		return ErrEmpty
	case errors.Is(err, imageutil.ErrTooLarge):
		return ErrImageTooLarge
	case errors.Is(err, imageutil.ErrNotImage):
		return ErrInvalidImage
	default:
		return fmt.Errorf("photo: process: %w", err)
	}
}

// sanitizeName очищает имя файла от пути и ограничивает длину — оно
// уходит и в метаданные, и в имена внутри архива.
func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		name = "photo"
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
