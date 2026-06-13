// Package imageutil обрабатывает загружаемые изображения профиля:
// проверяет их подлинность, защищается от decompression bomb, ужимает
// «тяжёлые» снимки и генерирует квадратный аватар.
//
// Три акцента, важных для продакшна (и для защиты лабы):
//
//  1. Анти-спуфинг. Формат определяется РЕАЛЬНОЙ попыткой декодирования
//     (image.DecodeConfig/Decode), а не по расширению файла или заголовку
//     Content-Type, который шлёт клиент. Файл с расширением .png, внутри
//     которого исполняемый код, просто не декодируется и отклоняется.
//
//  2. Защита от decompression bomb. Габариты читаются из заголовка
//     (DecodeConfig — без полной распаковки в память) и проверяются против
//     MaxPixels ДО декодирования: крошечный по байтам PNG может
//     разворачиваться в гигапиксели и исчерпать память сервера.
//
//  3. Сжатие. Снимок крупнее MaxStoredDimension (например, 4K 3840×2160)
//     ужимается по большей стороне с сохранением пропорций и
//     перекодируется — хранимая «оригинальная» копия всегда сжата.
package imageutil

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // регистрирует webp-декодер для image.Decode/DecodeConfig
)

const (
	// MaxPixels — потолок «площади» (ширина×высота). 40 Мп с запасом
	// покрывает реальные снимки (4K ≈ 8 Мп, 8K ≈ 33 Мп), но отсекает
	// заведомо вредоносные изображения-бомбы.
	MaxPixels = 40_000_000

	// MaxStoredDimension — максимальная сторона хранимого оригинала, px.
	// Снимок крупнее ужимается по большей стороне до этого значения.
	MaxStoredDimension = 1600

	// AvatarSize — сторона квадратного аватара, px (требование ТЗ — 128×128).
	AvatarSize = 128

	// jpegQuality — качество JPEG для перекодированных копий: баланс между
	// «визуально без потерь» и заметным сжатием.
	jpegQuality = 82
)

// Ошибки обработки изображения.
var (
	// ErrEmpty — на вход пришёл пустой буфер.
	ErrEmpty = errors.New("imageutil: пустой файл")
	// ErrNotImage — файл не является изображением поддерживаемого формата
	// (jpeg/png/webp). В том числе случай подмены расширения.
	ErrNotImage = errors.New("imageutil: файл не является изображением (jpeg/png/webp)")
	// ErrTooLarge — разрешение изображения превышает MaxPixels.
	ErrTooLarge = errors.New("imageutil: слишком большое разрешение изображения")
)

// Processed — производные одного загруженного снимка, готовые к хранению.
type Processed struct {
	// Original — сжатая копия оригинала (даунскейл + перекодирование).
	Original            []byte
	OriginalContentType string // image/jpeg | image/png
	Format              string // jpeg | png (нормализованный формат хранения)
	Width               int    // размеры хранимого оригинала, px
	Height              int
	// Avatar — квадрат AvatarSize×AvatarSize.
	Avatar            []byte
	AvatarContentType string
}

// Process валидирует входной снимок и готовит сжатый оригинал и аватар.
//
// Порядок проверок намеренно «дешёвое → дорогое»: сначала заголовок
// (DecodeConfig) и лимит пикселей, и только потом полная распаковка.
// Возвращает ErrEmpty / ErrNotImage / ErrTooLarge на невалидном входе.
func Process(raw []byte) (*Processed, error) {
	if len(raw) == 0 {
		return nil, ErrEmpty
	}

	// 1. Заголовок без распаковки: и тип (анти-спуфинг), и габариты (бомба).
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrNotImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, ErrNotImage
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, fmt.Errorf("%w: %dx%d", ErrTooLarge, cfg.Width, cfg.Height)
	}

	// 2. Полная распаковка. Любая ошибка здесь — тоже признак того, что
	// «изображение» на самом деле битое/поддельное.
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrNotImage
	}

	// JPEG не умеет прозрачность: если у исходника есть альфа-канал, копии
	// кодируем в PNG (без потерь), иначе — в JPEG (сильнее сжимает фото).
	preferPNG := !isOpaque(src)

	original := downscaleToFit(src, MaxStoredDimension)
	origBytes, origCT, format, err := encode(original, preferPNG)
	if err != nil {
		return nil, err
	}

	avatar := squareThumbnail(src, AvatarSize)
	avatarBytes, avatarCT, _, err := encode(avatar, preferPNG)
	if err != nil {
		return nil, err
	}

	b := original.Bounds()
	return &Processed{
		Original:            origBytes,
		OriginalContentType: origCT,
		Format:              format,
		Width:               b.Dx(),
		Height:              b.Dy(),
		Avatar:              avatarBytes,
		AvatarContentType:   avatarCT,
	}, nil
}

// downscaleToFit ужимает изображение так, чтобы большая сторона не
// превышала maxSide, сохраняя пропорции. Если снимок и так умещается —
// возвращает исходник без копирования.
func downscaleToFit(src image.Image, maxSide int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxSide && h <= maxSide {
		return src
	}

	var nw, nh int
	if w >= h {
		nw = maxSide
		nh = int(float64(h) * float64(maxSide) / float64(w))
	} else {
		nh = maxSide
		nw = int(float64(w) * float64(maxSide) / float64(h))
	}
	nw = max(nw, 1)
	nh = max(nh, 1)

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// squareThumbnail вырезает центрированный квадрат по меньшей стороне и
// масштабирует его до size×size (стратегия cover — миниатюра без полей).
func squareThumbnail(src image.Image, size int) image.Image {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	offX := b.Min.X + (b.Dx()-side)/2
	offY := b.Min.Y + (b.Dy()-side)/2
	square := image.Rect(offX, offY, offX+side, offY+side)

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, square, draw.Over, nil)
	return dst
}

// encode сериализует изображение: PNG (без потерь) при наличии альфы,
// иначе JPEG с jpegQuality. Возвращает байты, MIME-тип и слаг формата.
func encode(img image.Image, preferPNG bool) (data []byte, contentType, format string, err error) {
	var buf bytes.Buffer
	if preferPNG {
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, img); err != nil {
			return nil, "", "", fmt.Errorf("imageutil: encode png: %w", err)
		}
		return buf.Bytes(), "image/png", "png", nil
	}
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, "", "", fmt.Errorf("imageutil: encode jpeg: %w", err)
	}
	return buf.Bytes(), "image/jpeg", "jpeg", nil
}

// isOpaque сообщает, полностью ли непрозрачно изображение. Типы из
// стандартной библиотеки с возможной альфой (*image.RGBA, *image.NRGBA,
// *image.Paletted, …) реализуют Opaque(); YCbCr/Gray (типичный результат
// декода JPEG) альфы не имеют и считаются непрозрачными.
func isOpaque(img image.Image) bool {
	type opaquer interface{ Opaque() bool }
	if o, ok := img.(opaquer); ok {
		return o.Opaque()
	}
	return true
}
