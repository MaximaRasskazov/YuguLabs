package attendance

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Ошибки парсинга. ErrMissingColumns/ErrSheetNotFound — структурные
// (на handler-слое → 422). ErrRowInvalid оборачивает проблему конкретной
// строки и тоже маппится в 422 с пояснением, какая строка виновата.
var (
	// ErrSheetNotFound — в книге нет листа "Посещаемость".
	ErrSheetNotFound = errors.New("attendance: лист «" + SheetName + "» не найден")
	// ErrMissingColumns — отсутствуют обязательные колонки.
	ErrMissingColumns = errors.New("attendance: в файле нет обязательных колонок")
	// ErrRowInvalid — некорректные данные в конкретной строке.
	ErrRowInvalid = errors.New("attendance: некорректная строка")
	// ErrTooManyRows — в листе слишком много строк (защита от «xlsx-бомбы»:
	// маленький по размеру файл, разворачивающийся в миллионы строк).
	ErrTooManyRows = errors.New("attendance: в листе слишком много строк")
	// ErrNotXLSX — файл не проходит проверку по сигнатуре: первые байты
	// не соответствуют формату ZIP/XLSX (PK\x03\x04). Расширение файла
	// не проверяется — оно не является надёжным признаком формата.
	ErrNotXLSX = errors.New("attendance: файл не является XLSX (неверная сигнатура формата)")
)

// xlsxMagic — первые четыре байта любого XLSX-файла (ZIP PK-сигнатура).
// XLSX — это ZIP-архив; файл с любым расширением, чьё содержимое не начинается
// с этих байт, заведомо не является книгой Excel.
var xlsxMagic = []byte{0x50, 0x4B, 0x03, 0x04}

// maxDataRows — потолок числа строк данных (без заголовка), которые мы готовы
// обработать. Лимит по размеру файла (UPLOAD_MAX_SIZE_MB) не спасает от
// «бомбы»: сжатый xlsx — это zip, и крошечный архив разворачивается в гигантскую
// таблицу. Поэтому отдельно ограничиваем число строк. 200 000 ≈ запас на 4–5
// учебных потоков; всё, что больше, — почти наверняка ошибка или атака.
//
// var (а не const) — чтобы тест мог временно понизить лимит и проверить отсечку
// без генерации 200k строк. В рантайме значение не меняется.
var maxDataRows = 200_000

// dateRe проверяет формат даты ДД.ММ.ГГГГ. Строгая валидация формата (а не
// только парсинг числа) — чтобы «23-09-2024» отвергалось предсказуемо (TC-26).
var dateRe = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}$`)

// timeRe проверяет формат времени ЧЧ:ММ.
var timeRe = regexp.MustCompile(`^\d{2}:\d{2}$`)

// Parser читает .xlsx и нормализует строки листа "Посещаемость" в []RawRow.
// Stateless — можно переиспользовать между запросами.
type Parser struct{}

// NewParser создаёт парсер.
func NewParser() *Parser { return &Parser{} }

// Parse открывает книгу из байтов, находит лист, сопоставляет колонки по
// заголовкам и нормализует строки данных. Возвращает []RawRow либо ошибку:
//   - ErrSheetNotFound — нет нужного листа;
//   - ErrMissingColumns — нет обязательных колонок;
//   - ErrRowInvalid (обёрнутая) — битая строка данных.
//
// Пустой файл (только заголовки) — не ошибка: вернётся пустой срез.
func (p *Parser) Parse(content []byte) ([]RawRow, error) {
	if len(content) < 4 || !bytes.HasPrefix(content, xlsxMagic) {
		return nil, ErrNotXLSX
	}
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("attendance: не удалось открыть .xlsx: %w", err)
	}
	defer func() { _ = f.Close() }()

	if !sheetExists(f, SheetName) {
		return nil, ErrSheetNotFound
	}

	rows, err := f.GetRows(SheetName)
	if err != nil {
		return nil, fmt.Errorf("attendance: чтение листа: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrSheetNotFound
	}
	// Защита от «xlsx-бомбы»: число строк данных (без заголовка) не должно
	// превышать потолок. Проверяем ДО нормализации, чтобы не делать лишней
	// работы на заведомо неподъёмном файле.
	if len(rows)-1 > maxDataRows {
		return nil, fmt.Errorf("%w: %d (максимум %d)", ErrTooManyRows, len(rows)-1, maxDataRows)
	}

	idx, err := mapColumns(rows[0])
	if err != nil {
		return nil, err
	}

	out := make([]RawRow, 0, len(rows)-1)
	for i, cells := range rows[1:] {
		rowNum := i + 2 // +1 за 0-based, +1 за строку заголовка
		if isEmptyRow(cells) {
			continue // полностью пустые строки пропускаем молча
		}
		row, err := normalizeRow(cells, idx, rowNum)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

// sheetExists — есть ли в книге лист с таким именем (регистр учитывается, как в Excel).
func sheetExists(f *excelize.File, name string) bool {
	for _, s := range f.GetSheetList() {
		if s == name {
			return true
		}
	}
	return false
}

// columnIndex — позиции нужных колонок в строке заголовка. -1 = колонки нет.
type columnIndex struct {
	group, subgroup, fullName, date, time, typ, number, visit, doneLabs, autoCredit int
}

// mapColumns строит columnIndex по строке заголовка. Имена нормализуются
// (trim + lower), чтобы «ФИО » и «фио» считались одной колонкой. Ошибка, если
// не найдена хотя бы одна обязательная колонка.
func mapColumns(header []string) (columnIndex, error) {
	pos := make(map[string]int, len(header))
	for i, h := range header {
		pos[normalizeHeader(h)] = i
	}
	find := func(name string) int {
		if i, ok := pos[normalizeHeader(name)]; ok {
			return i
		}
		return -1
	}

	idx := columnIndex{
		group:      find(colGroup),
		subgroup:   find(colSubgroup),
		fullName:   find(colFullName),
		date:       find(colDate),
		time:       find(colTime),
		typ:        find(colType),
		number:     find(colNumber),
		visit:      find(colVisit),
		doneLabs:   find(colDoneLabs),
		autoCredit: find(colAutoCredit),
	}

	var missing []string
	for _, name := range requiredColumns {
		if find(name) == -1 {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return columnIndex{}, fmt.Errorf("%w: %s", ErrMissingColumns, strings.Join(missing, ", "))
	}
	return idx, nil
}

// normalizeRow приводит ячейки одной строки к RawRow с валидацией.
func normalizeRow(cells []string, idx columnIndex, rowNum int) (RawRow, error) {
	get := func(col int) string {
		if col < 0 || col >= len(cells) {
			return ""
		}
		return strings.TrimSpace(cells[col])
	}

	fullName := get(idx.fullName)
	if fullName == "" {
		return RawRow{}, rowErr(rowNum, "пустое ФИО")
	}

	group := get(idx.group)
	if group == "" {
		return RawRow{}, rowErr(rowNum, "пустая группа")
	}

	date := get(idx.date)
	if !dateRe.MatchString(date) {
		return RawRow{}, rowErr(rowNum, "дата должна быть в формате ДД.ММ.ГГГГ, получено: "+quote(date))
	}

	tm := get(idx.time)
	if !timeRe.MatchString(tm) {
		return RawRow{}, rowErr(rowNum, "время должно быть в формате ЧЧ:ММ, получено: "+quote(tm))
	}

	typ := strings.ToLower(get(idx.typ))
	if typ != "lect" && typ != "lab" {
		return RawRow{}, rowErr(rowNum, `тип занятия должен быть "lect" или "lab", получено: `+quote(typ))
	}

	number, err := parseInt(get(idx.number))
	if err != nil {
		return RawRow{}, rowErr(rowNum, "номер занятия — не целое число: "+quote(get(idx.number)))
	}

	doneLabs, err := parseInt(get(idx.doneLabs))
	if err != nil {
		return RawRow{}, rowErr(rowNum, "«Выполнено лаб» — не целое число: "+quote(get(idx.doneLabs)))
	}
	if doneLabs < 0 {
		return RawRow{}, rowErr(rowNum, "«Выполнено лаб» не может быть отрицательным")
	}

	visit, err := parseFlag(get(idx.visit))
	if err != nil {
		return RawRow{}, rowErr(rowNum, "«Посещение» должно быть 0 или 1: "+quote(get(idx.visit)))
	}

	// Подгруппа необязательна: пустая → 1.
	subgroup := 1
	if raw := get(idx.subgroup); raw != "" {
		subgroup, err = parseInt(raw)
		if err != nil || subgroup < 1 {
			return RawRow{}, rowErr(rowNum, "подгруппа должна быть целым ≥ 1: "+quote(raw))
		}
	}

	// Авто-зачёт необязателен: пустой → false.
	autoCredit := false
	if raw := get(idx.autoCredit); raw != "" {
		autoCredit, err = parseFlag(raw)
		if err != nil {
			return RawRow{}, rowErr(rowNum, "«Зачёт автоматом» должно быть 0 или 1: "+quote(raw))
		}
	}

	return RawRow{
		Group:      group,
		Subgroup:   subgroup,
		FullName:   fullName,
		Date:       date,
		Time:       tm,
		Type:       typ,
		Number:     number,
		Visited:    visit,
		DoneLabs:   doneLabs,
		AutoCredit: autoCredit,
		rowNum:     rowNum,
	}, nil
}

// normalizeHeader приводит заголовок к каноничному виду для сравнения:
// trim, lower и унификация «ё»→«е» (чтобы «Зачет»/«Зачёт» совпадали).
func normalizeHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ё", "е")
	return s
}

// parseInt парсит целое, допуская десятичный «.0» (Excel часто отдаёт числа
// как "1.0"). Пустая строка — ошибка (вызывающий решает, фатально ли это).
func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i, nil
	}
	// Фолбэк на "1.0" / "1,0".
	s = strings.ReplaceAll(s, ",", ".")
	if f, err := strconv.ParseFloat(s, 64); err == nil && f == float64(int(f)) {
		return int(f), nil
	}
	return 0, fmt.Errorf("not an integer: %q", s)
}

// parseFlag интерпретирует значение колонки-флага (Посещение / Зачёт автоматом).
// Принимает 1/0, true/false, да/нет, yes/no — устойчивость к «человеческому»
// заполнению файла (TC-27). Иначе — ошибка.
func parseFlag(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "1.0", "true", "да", "yes", "y", "+":
		return true, nil
	case "0", "0.0", "false", "нет", "no", "n", "-", "":
		return false, nil
	default:
		return false, fmt.Errorf("not a flag: %q", s)
	}
}

// isEmptyRow — true, если все ячейки строки пустые (после trim).
func isEmptyRow(cells []string) bool {
	for _, c := range cells {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// rowErr собирает ошибку конкретной строки, оборачивая ErrRowInvalid.
func rowErr(rowNum int, msg string) error {
	return fmt.Errorf("%w (строка %d): %s", ErrRowInvalid, rowNum, msg)
}

// quote оборачивает значение в кавычки для сообщений об ошибках.
func quote(s string) string {
	if s == "" {
		return "(пусто)"
	}
	return "«" + s + "»"
}
