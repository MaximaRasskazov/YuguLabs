package photo

// Архивная выгрузка всех активных фотографий для администратора:
//   originals/{base}.{ext}       — сжатые оригиналы
//   avatars/{base}_avatar.{ext}  — миниатюры 128×128
//   registry.xlsx                — реестр (см. заголовки headers)
// где base = {локальная_часть_email}_{photoID}. photoID уникален, поэтому
// имена в архиве не конфликтуют, даже если ФИО двух пользователей совпадают.
//
// Мягко удалённые фотографии в архив не попадают (запрос фильтрует
// deleted_at IS NULL).

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
)

// archiveSheet — имя листа Excel-реестра.
const archiveSheet = "Фотографии"

// Archive — готовый ZIP-архив для отдачи администратору.
type Archive struct {
	Filename string
	Content  []byte
}

// BuildArchive собирает ZIP со всеми активными фотографиями пользователей
// и Excel-реестром. Хранилище — БД, поэтому в реестре указывается
// логический локатор (db://…), а не путь в файловой системе.
func (s *Service) BuildArchive(ctx context.Context) (*Archive, error) {
	rows, err := s.store.ListActiveUserPhotosForArchive(ctx)
	if err != nil {
		return nil, fmt.Errorf("photo: list for archive: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	xl := excelize.NewFile()
	defer func() { _ = xl.Close() }()
	xl.SetSheetName(xl.GetSheetName(0), archiveSheet)

	headers := []string{
		"ID пользователя", "Имя пользователя", "ФИО", "Группа",
		"Дата загрузки", "Файл в архиве", "Путь на сервере",
	}
	if err := setRow(xl, 1, headers); err != nil {
		return nil, err
	}

	for idx, r := range rows {
		ext := formatExt(r.Format)
		avExt := contentTypeExt(r.AvatarContentType)
		base := fmt.Sprintf("%s_%d", sanitizeName(localPart(r.Email)), r.ID)
		archiveFile := fmt.Sprintf("%s.%s", base, ext)

		if err := writeZipEntry(zw, "originals/"+archiveFile, r.OriginalContent); err != nil {
			return nil, err
		}
		if err := writeZipEntry(zw, fmt.Sprintf("avatars/%s_avatar.%s", base, avExt), r.AvatarContent); err != nil {
			return nil, err
		}

		fio := strings.Join(nonEmpty(r.LastName, r.FirstName, deref(r.MiddleName)), " ")
		// В БД нет файлового пути — даём честный логический локатор, по которому
		// видно, где лежит оригинал (таблица/строка), до копирования в архив.
		serverPath := fmt.Sprintf("db://user_photos/%d/original", r.ID)
		values := []any{
			pgutil.UUID(r.UserID).String(),
			r.Email,
			fio,
			deref(r.GroupName),
			r.CreatedAt.Time.Format("2006-01-02 15:04:05"),
			archiveFile,
			serverPath,
		}
		if err := setRow(xl, idx+2, values); err != nil {
			return nil, err
		}
	}

	xlBuf, err := xl.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("photo: write xlsx: %w", err)
	}
	if err := writeZipEntry(zw, "registry.xlsx", xlBuf.Bytes()); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("photo: close zip: %w", err)
	}

	return &Archive{
		Filename: fmt.Sprintf("user-photos-%s.zip", time.Now().Format("20060102-150405")),
		Content:  buf.Bytes(),
	}, nil
}

// setRow записывает срез значений в строку rowNum (1-based) листа реестра.
func setRow[T any](xl *excelize.File, rowNum int, values []T) error {
	for col, v := range values {
		cell, err := excelize.CoordinatesToCellName(col+1, rowNum)
		if err != nil {
			return fmt.Errorf("photo: cell name: %w", err)
		}
		if err := xl.SetCellValue(archiveSheet, cell, v); err != nil {
			return fmt.Errorf("photo: set cell: %w", err)
		}
	}
	return nil
}

// writeZipEntry добавляет один файл в ZIP. Используется Deflate по умолчанию
// (zip.Writer), что для уже сжатых JPEG/PNG почти не даёт выигрыша, но
// сохраняет единый формат архива.
func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("photo: zip create %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("photo: zip write %s: %w", name, err)
	}
	return nil
}

// formatExt переводит слаг формата хранения в расширение файла.
func formatExt(format string) string {
	if format == "jpeg" {
		return "jpg"
	}
	return format
}

// contentTypeExt переводит MIME-тип в расширение файла.
func contentTypeExt(contentType string) string {
	switch contentType {
	case "image/png":
		return "png"
	default:
		return "jpg"
	}
}

// localPart возвращает часть email до «@» (логин пользователя).
func localPart(email string) string {
	if i := strings.IndexByte(email, '@'); i >= 0 {
		return email[:i]
	}
	return email
}

// deref разыменовывает *string, возвращая "" для nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nonEmpty отбрасывает пустые строки (после trim) — для сборки ФИО без
// лишних пробелов, когда отчество отсутствует.
func nonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}
