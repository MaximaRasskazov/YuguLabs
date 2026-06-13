package imageutil_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/imageutil"
)

// makeJPEG создаёт валидный JPEG w×h с градиентной заливкой (не вырожденный,
// чтобы кодек отработал штатно). YCbCr-результат декода — непрозрачный.
func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: uint8((x + y) % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

// makeAlphaPNG создаёт PNG с альфа-каналом (есть полностью прозрачный пиксель).
func makeAlphaPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(y % 256), B: 100, A: 255})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{}) // прозрачный пиксель → изображение не opaque
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// pngHeaderOnly собирает PNG-сигнатуру и корректный (с CRC) чанк IHDR с
// заданными размерами, БЕЗ пиксельных данных. image.DecodeConfig читает
// только IHDR, поэтому этого достаточно, чтобы проверить защиту от
// decompression bomb без аллокации гигапикселей в памяти.
func pngHeaderOnly(w, h int) []byte {
	sig := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

	data := make([]byte, 0, 13)
	data = binary.BigEndian.AppendUint32(data, uint32(w))
	data = binary.BigEndian.AppendUint32(data, uint32(h))
	data = append(data, 8, 2, 0, 0, 0) // bit depth 8, truecolor, без сжатия/фильтра/интерлейса

	typeAndData := append([]byte("IHDR"), data...)

	var chunk bytes.Buffer
	_ = binary.Write(&chunk, binary.BigEndian, uint32(len(data)))
	chunk.Write(typeAndData)
	_ = binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(typeAndData))

	return append(sig, chunk.Bytes()...)
}

func TestProcess_RejectsEmpty(t *testing.T) {
	_, err := imageutil.Process(nil)
	require.ErrorIs(t, err, imageutil.ErrEmpty)
}

// Сценарий «подмена расширения»: байты не являются изображением, хотя клиент
// мог прислать их как image/png с именем .png. Реальный декод отклоняет файл.
func TestProcess_RejectsSpoofedImage(t *testing.T) {
	_, err := imageutil.Process([]byte("Это не картинка, несмотря на расширение .png и заголовок image/png"))
	require.ErrorIs(t, err, imageutil.ErrNotImage)
}

func TestProcess_RejectsTruncatedImage(t *testing.T) {
	raw := makeJPEG(t, 64, 64)
	_, err := imageutil.Process(raw[:20]) // обрезанный JPEG не декодируется целиком
	require.ErrorIs(t, err, imageutil.ErrNotImage)
}

// 4K-снимок ужимается по большей стороне до MaxStoredDimension с сохранением
// пропорций, а аватар всегда 128×128.
func TestProcess_DownscalesLargeAndBuildsAvatar(t *testing.T) {
	raw := makeJPEG(t, 4000, 3000) // 12 Мп < MaxPixels
	p, err := imageutil.Process(raw)
	require.NoError(t, err)

	require.Equal(t, imageutil.MaxStoredDimension, p.Width, "большая сторона ужата до лимита")
	require.Equal(t, 1200, p.Height, "пропорции 4:3 сохранены")
	require.Equal(t, "image/jpeg", p.OriginalContentType)
	require.Equal(t, "jpeg", p.Format)

	// Хранимый оригинал сжат (заметно меньше исходного 4000×3000 JPEG).
	require.Less(t, len(p.Original), len(raw))

	orig, _, err := image.Decode(bytes.NewReader(p.Original))
	require.NoError(t, err)
	require.Equal(t, imageutil.MaxStoredDimension, orig.Bounds().Dx())

	avatar, _, err := image.Decode(bytes.NewReader(p.Avatar))
	require.NoError(t, err)
	require.Equal(t, imageutil.AvatarSize, avatar.Bounds().Dx())
	require.Equal(t, imageutil.AvatarSize, avatar.Bounds().Dy())
}

func TestProcess_SmallImageNotUpscaled(t *testing.T) {
	raw := makeJPEG(t, 100, 80)
	p, err := imageutil.Process(raw)
	require.NoError(t, err)
	require.Equal(t, 100, p.Width)
	require.Equal(t, 80, p.Height)
}

// Изображение с альфа-каналом хранится как PNG (JPEG не умеет прозрачность).
func TestProcess_AlphaImageStaysPNG(t *testing.T) {
	raw := makeAlphaPNG(t, 256, 256)
	p, err := imageutil.Process(raw)
	require.NoError(t, err)
	require.Equal(t, "image/png", p.OriginalContentType)
	require.Equal(t, "png", p.Format)
	require.Equal(t, "image/png", p.AvatarContentType)
}

// Защита от decompression bomb: огромные габариты из заголовка отклоняются
// до полной распаковки.
func TestProcess_RejectsPixelBomb(t *testing.T) {
	raw := pngHeaderOnly(40000, 40000) // 1.6 млрд пикселей > MaxPixels
	_, err := imageutil.Process(raw)
	require.ErrorIs(t, err, imageutil.ErrTooLarge)
}
