package photo_test

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/photo"
)

// testStore поднимает реальный Store на TEST_DATABASE_URL. Без него
// интеграционный тест пропускается — единый подход со всеми сервисами.
func testStore(t *testing.T) *repo.Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))
	t.Cleanup(pool.Close)
	return repo.NewStore(pool)
}

func seedUser(t *testing.T, s *repo.Store) uuid.UUID {
	t.Helper()
	email := "photo+" + time.Now().Format("150405.000000000") + "@test.local"
	u, err := s.CreateUser(context.Background(), queries.CreateUserParams{
		Email:        email,
		PasswordHash: "$2a$10$placeholder",
		FirstName:    "Иван",
		LastName:     "Петров",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.CleanupUser(context.Background(), s.Pool(), u.ID) })
	return pgutil.UUID(u.ID)
}

// jpegBytes генерирует валидный JPEG заданного размера.
func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func TestUpload_RejectsSpoofedFile(t *testing.T) {
	s := testStore(t)
	svc := photo.New(s)
	uid := seedUser(t, s)

	_, err := svc.Upload(context.Background(), uid, []byte("не картинка, расширение .png"), "evil.png", "", uid)
	require.ErrorIs(t, err, photo.ErrInvalidImage)

	// Не должна была создаться запись.
	_, err = svc.Info(context.Background(), uid)
	require.ErrorIs(t, err, photo.ErrNotFound)
}

func TestUpload_RejectsEmpty(t *testing.T) {
	s := testStore(t)
	svc := photo.New(s)
	uid := seedUser(t, s)

	_, err := svc.Upload(context.Background(), uid, nil, "x.jpg", "", uid)
	require.ErrorIs(t, err, photo.ErrEmpty)
}

func TestUpload_StoresOriginalAndAvatar(t *testing.T) {
	s := testStore(t)
	svc := photo.New(s)
	uid := seedUser(t, s)

	info, err := svc.Upload(context.Background(), uid, jpegBytes(t, 800, 600), "me.jpg", "моё фото", uid)
	require.NoError(t, err)
	require.Equal(t, "me.jpg", info.OriginalName)
	require.NotNil(t, info.Description)
	require.Equal(t, "jpeg", info.Format)
	require.Positive(t, info.SizeBytes)

	// Аватар — квадрат 128×128.
	av, err := svc.Avatar(context.Background(), uid)
	require.NoError(t, err)
	decoded, _, err := image.Decode(bytes.NewReader(av.Content))
	require.NoError(t, err)
	require.Equal(t, 128, decoded.Bounds().Dx())
	require.Equal(t, 128, decoded.Bounds().Dy())

	// Оригинал доступен для скачивания.
	orig, err := svc.Original(context.Background(), uid)
	require.NoError(t, err)
	require.NotEmpty(t, orig.Content)
}

// Повторная загрузка оставляет ровно одну активную фотографию (прежняя
// мягко удаляется).
func TestUpload_ReplacesPrevious(t *testing.T) {
	s := testStore(t)
	svc := photo.New(s)
	uid := seedUser(t, s)

	first, err := svc.Upload(context.Background(), uid, jpegBytes(t, 200, 200), "a.jpg", "", uid)
	require.NoError(t, err)
	second, err := svc.Upload(context.Background(), uid, jpegBytes(t, 300, 200), "b.jpg", "", uid)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)

	info, err := svc.Info(context.Background(), uid)
	require.NoError(t, err)
	require.Equal(t, second.ID, info.ID, "активной остаётся последняя загруженная")
}

func TestDelete_RemovesActivePhoto(t *testing.T) {
	s := testStore(t)
	svc := photo.New(s)
	uid := seedUser(t, s)

	_, err := svc.Upload(context.Background(), uid, jpegBytes(t, 200, 200), "a.jpg", "", uid)
	require.NoError(t, err)

	require.NoError(t, svc.Delete(context.Background(), uid, uid))

	_, err = svc.Info(context.Background(), uid)
	require.ErrorIs(t, err, photo.ErrNotFound)

	// Повторное удаление идемпотентно.
	require.NoError(t, svc.Delete(context.Background(), uid, uid))
}

func TestListAll_ReturnsActivePhotosWithOwner(t *testing.T) {
	s := testStore(t)
	svc := photo.New(s)
	uid := seedUser(t, s)

	_, err := svc.Upload(context.Background(), uid, jpegBytes(t, 300, 300), "me.jpg", "", uid)
	require.NoError(t, err)

	all, err := svc.ListAll(context.Background())
	require.NoError(t, err)

	var found bool
	for _, p := range all {
		if p.UserID == uid {
			found = true
			require.NotEmpty(t, p.Email)
			require.NotEmpty(t, p.FullName)
			require.Equal(t, "me.jpg", p.OriginalName)
			require.Equal(t, "jpeg", p.Format)
			require.Positive(t, p.SizeBytes)
		}
	}
	require.True(t, found, "загруженное фото присутствует в списке всех")
}

func TestBuildArchive_ContainsRegistryAndFiles(t *testing.T) {
	s := testStore(t)
	svc := photo.New(s)
	uid := seedUser(t, s)

	_, err := svc.Upload(context.Background(), uid, jpegBytes(t, 400, 400), "me.jpg", "", uid)
	require.NoError(t, err)

	arc, err := svc.BuildArchive(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, arc.Content)

	zr, err := zip.NewReader(bytes.NewReader(arc.Content), int64(len(arc.Content)))
	require.NoError(t, err)

	var hasRegistry, hasOriginal, hasAvatar bool
	for _, f := range zr.File {
		switch {
		case f.Name == "registry.xlsx":
			hasRegistry = true
		case len(f.Name) > len("originals/") && f.Name[:len("originals/")] == "originals/":
			hasOriginal = true
		case len(f.Name) > len("avatars/") && f.Name[:len("avatars/")] == "avatars/":
			hasAvatar = true
		}
	}
	require.True(t, hasRegistry, "в архиве есть Excel-реестр")
	require.True(t, hasOriginal, "в архиве есть оригиналы")
	require.True(t, hasAvatar, "в архиве есть аватары")
}
