package attendance_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/attendance"
)

// header — стандартная строка заголовков по ТЗ. Колонки идут в «каноничном»
// порядке, но Parser ищет их по имени, что отдельно проверяется в TestParser_ColumnOrderIndependent.
var header = []string{
	"Группа", "Подгруппа", "ФИО", "Дата", "Время",
	"Тип занятия", "Номер занятия", "Посещение", "Выполнено лаб", "Зачёт автоматом",
}

// buildXLSX собирает .xlsx в памяти: лист sheet + переданные строки данных
// (заголовок добавляется автоматически, если addHeader=true).
func buildXLSX(t *testing.T, sheet string, addHeader bool, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	idx, err := f.NewSheet(sheet)
	require.NoError(t, err)
	f.SetActiveSheet(idx)
	require.NoError(t, f.DeleteSheet("Sheet1"))

	rowNum := 1
	if addHeader {
		for col, h := range header {
			cell, err := excelize.CoordinatesToCellName(col+1, rowNum)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(sheet, cell, h))
		}
		rowNum++
	}
	for _, r := range rows {
		for col, v := range r {
			cell, err := excelize.CoordinatesToCellName(col+1, rowNum)
			require.NoError(t, err)
			require.NoError(t, f.SetCellValue(sheet, cell, v))
		}
		rowNum++
	}

	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))
	return buf.Bytes()
}

// row — удобный конструктор строки данных в порядке header.
func row(group string, sub any, name, date, tm, typ string, num, visit, labs, auto any) []any {
	return []any{group, sub, name, date, tm, typ, num, visit, labs, auto}
}

// newSvc — сервис с правилами по умолчанию из ТЗ: 5 лаб, порог 80%.
func newSvc() *attendance.Service { return attendance.New(5, 80) }

// findStudent ищет студента по ФИО в первой группе ответа.
func findStudent(t *testing.T, rep attendance.Report, group, name string) attendance.StudentResult {
	t.Helper()
	for _, g := range rep.Groups {
		if g.GroupName != group {
			continue
		}
		for _, s := range g.Students {
			if s.Name == name {
				return s
			}
		}
	}
	t.Fatalf("студент %q в группе %q не найден", name, group)
	return attendance.StudentResult{}
}

// TestCalculate_Success — базовый успешный расчёт: студент с посещаемостью
// ≥80% и 5 лабами получает зачёт; структура JSON соответствует ТЗ.
func TestCalculate_Success(t *testing.T) {
	// 5 уникальных занятий, студент посетил 4 (80%), лаб=5 → зачёт.
	rows := [][]any{
		row("101б", 1, "Иванов Иван", "23.09.2024", "08:00", "lect", 1, 1, 5, 0),
		row("101б", 1, "Иванов Иван", "24.09.2024", "08:00", "lab", 1, 1, 5, 0),
		row("101б", 1, "Иванов Иван", "25.09.2024", "08:00", "lab", 2, 1, 5, 0),
		row("101б", 1, "Иванов Иван", "26.09.2024", "08:00", "lab", 3, 1, 5, 0),
		row("101б", 1, "Иванов Иван", "27.09.2024", "08:00", "lab", 4, 0, 5, 0),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	require.Len(t, rep.Groups, 1)

	g := rep.Groups[0]
	require.Equal(t, "101б", g.GroupName)
	require.Len(t, g.Students, 1)

	s := g.Students[0]
	require.Equal(t, "Иванов Иван", s.Name)
	require.Equal(t, 1, s.Subgroup)
	require.Len(t, s.Lessons, 5)
	require.Equal(t, 80, s.VisitPercent)
	require.Equal(t, 5, s.SuccessLabs)
	require.Equal(t, 100, s.SuccessLabsPercent)
	require.True(t, s.Result)

	require.Equal(t, 1, g.Result.Success)
	require.Equal(t, 0, g.Result.Unsuccessfully)

	// Доп.список зачётников.
	require.Len(t, rep.AutomaticSuccessStudents, 1)
	require.Equal(t, "Иванов Иван", rep.AutomaticSuccessStudents[0].Name)
	require.Equal(t, "101б", rep.AutomaticSuccessStudents[0].Group)
}

// TestCalculate_LessonsSortedByDateTime — занятия в leasons отсортированы по
// дате и времени по возрастанию (TC-13), независимо от порядка в файле.
func TestCalculate_LessonsSortedByDateTime(t *testing.T) {
	rows := [][]any{
		row("101б", 1, "Петров П", "25.09.2024", "10:00", "lab", 2, 1, 1, 0),
		row("101б", 1, "Петров П", "23.09.2024", "12:00", "lect", 1, 1, 1, 0),
		row("101б", 1, "Петров П", "23.09.2024", "08:00", "lab", 1, 1, 1, 0),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	s := findStudent(t, rep, "101б", "Петров П")
	require.Len(t, s.Lessons, 3)
	require.Equal(t, "23.09.2024", s.Lessons[0].Date)
	require.Equal(t, "08:00", s.Lessons[0].Time)
	require.Equal(t, "23.09.2024", s.Lessons[1].Date)
	require.Equal(t, "12:00", s.Lessons[1].Time)
	require.Equal(t, "25.09.2024", s.Lessons[2].Date)
}

// TestCalculate_EmptySubgroupDefaultsTo1 — пустая подгруппа трактуется как 1 (TC-15).
func TestCalculate_EmptySubgroupDefaultsTo1(t *testing.T) {
	rows := [][]any{
		row("101б", "", "Сидоров С", "23.09.2024", "08:00", "lab", 1, 1, 5, 0),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	s := findStudent(t, rep, "101б", "Сидоров С")
	require.Equal(t, 1, s.Subgroup)
	require.Equal(t, 1, s.Lessons[0].Subgroup)
}

// TestCalculate_AutoCreditWins — авто-зачёт даёт result=true даже при нулевой
// посещаемости и недостатке лаб (TC-17).
func TestCalculate_AutoCreditWins(t *testing.T) {
	rows := [][]any{
		row("101б", 1, "Лентяев Л", "23.09.2024", "08:00", "lab", 1, 0, 0, 1),
		row("101б", 1, "Лентяев Л", "24.09.2024", "08:00", "lab", 2, 0, 0, 1),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	s := findStudent(t, rep, "101б", "Лентяев Л")
	require.Equal(t, 0, s.VisitPercent)
	require.Equal(t, 0, s.SuccessLabs)
	require.True(t, s.Result, "авто-зачёт должен давать result=true")
}

// TestCalculate_Boundaries — граничные условия порога/лаб (TC-20/21/22).
func TestCalculate_Boundaries(t *testing.T) {
	cases := []struct {
		name        string
		visited     int // сколько из 5 занятий посетил
		labs        int
		wantResult  bool
		wantPercent int
	}{
		{"ровно порог и ровно лабы → зачёт", 4, 5, true, 80}, // 80% & 5 лаб
		{"посещаемость выше, лаб не хватает → нет", 5, 4, false, 100},
		{"посещаемость ниже порога, лаб хватает → нет", 3, 5, false, 60},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rows [][]any
			for i := 0; i < 5; i++ {
				visit := 0
				if i < tc.visited {
					visit = 1
				}
				rows = append(rows, row("101б", 1, "Студент С", "2"+itoa(i)+".09.2024", "08:00", "lab", i+1, visit, tc.labs, 0))
			}
			rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
			require.NoError(t, err)
			s := findStudent(t, rep, "101б", "Студент С")
			require.Equal(t, tc.wantPercent, s.VisitPercent)
			require.Equal(t, tc.wantResult, s.Result)
		})
	}
}

// itoa — маленький помощник: цифра 0..9 в строку (для дат вида 2X.09.2024).
func itoa(d int) string { return string(rune('0' + d)) }

// TestCalculate_SameLessonDifferentGroups — занятие с одинаковыми
// дата+время+тип+номер у разных групп считается ОДНИМ (TC-16): общее число
// уникальных занятий не растёт.
func TestCalculate_SameLessonDifferentGroups(t *testing.T) {
	rows := [][]any{
		row("101б", 1, "А А", "23.09.2024", "08:00", "lect", 1, 1, 1, 0),
		row("102б", 1, "Б Б", "23.09.2024", "08:00", "lect", 1, 1, 1, 0),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	require.Len(t, rep.Groups, 2)
	// У обоих по одному занятию (одно и то же), посещаемость 100%.
	a := findStudent(t, rep, "101б", "А А")
	b := findStudent(t, rep, "102б", "Б Б")
	require.Equal(t, 100, a.VisitPercent)
	require.Equal(t, 100, b.VisitPercent)
}

// TestCalculate_DuplicateRowCountedOnce — дубликат строки студента на одно
// занятие учитывается один раз (TC-28).
func TestCalculate_DuplicateRowCountedOnce(t *testing.T) {
	rows := [][]any{
		row("101б", 1, "Дубль Д", "23.09.2024", "08:00", "lab", 1, 1, 1, 0),
		row("101б", 1, "Дубль Д", "23.09.2024", "08:00", "lab", 1, 1, 1, 0),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	s := findStudent(t, rep, "101б", "Дубль Д")
	require.Len(t, s.Lessons, 1, "дубликаты занятия схлопываются")
	require.Equal(t, 100, s.VisitPercent)
}

// TestCalculate_DifferentLabsTakesMax — разные значения «Выполнено лаб» в
// строках одного студента: берётся максимум, без падения (TC-19).
func TestCalculate_DifferentLabsTakesMax(t *testing.T) {
	rows := [][]any{
		row("101б", 1, "Макс М", "23.09.2024", "08:00", "lab", 1, 1, 3, 0),
		row("101б", 1, "Макс М", "24.09.2024", "08:00", "lab", 2, 1, 5, 0),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	s := findStudent(t, rep, "101б", "Макс М")
	require.Equal(t, 5, s.SuccessLabs)
}

// TestCalculate_MultipleGroups — несколько групп в файле, у каждой свои итоги (TC-23).
func TestCalculate_MultipleGroups(t *testing.T) {
	rows := [][]any{
		row("101б", 1, "Отлич О", "23.09.2024", "08:00", "lab", 1, 1, 5, 0),
		row("102б", 1, "Двоеч Д", "23.09.2024", "08:00", "lab", 1, 0, 0, 0),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	require.Len(t, rep.Groups, 2)
	// Группы отсортированы по имени: 101б, затем 102б.
	require.Equal(t, "101б", rep.Groups[0].GroupName)
	require.Equal(t, 1, rep.Groups[0].Result.Success)
	require.Equal(t, "102б", rep.Groups[1].GroupName)
	require.Equal(t, 1, rep.Groups[1].Result.Unsuccessfully)
}

// TestCalculate_VisitPercentRounding — округление процента до целого (TC-14):
// 2 из 3 = 66.66% → 67.
func TestCalculate_VisitPercentRounding(t *testing.T) {
	rows := [][]any{
		row("101б", 1, "Дроб Д", "23.09.2024", "08:00", "lab", 1, 1, 1, 0),
		row("101б", 1, "Дроб Д", "24.09.2024", "08:00", "lab", 2, 1, 1, 0),
		row("101б", 1, "Дроб Д", "25.09.2024", "08:00", "lab", 3, 0, 1, 0),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	s := findStudent(t, rep, "101б", "Дроб Д")
	require.Equal(t, 67, s.VisitPercent)
}

// TestCalculate_EmptyFile — только заголовки, без данных: пустой, но валидный
// ответ (TC-25).
func TestCalculate_EmptyFile(t *testing.T) {
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, nil))
	require.NoError(t, err)
	require.Empty(t, rep.Groups)
	require.Empty(t, rep.AutomaticSuccessStudents)
}

// TestCalculate_ColumnOrderIndependent — колонки в произвольном порядке всё
// равно сопоставляются по имени.
func TestParser_ColumnOrderIndependent(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	idx, err := f.NewSheet(attendance.SheetName)
	require.NoError(t, err)
	f.SetActiveSheet(idx)
	require.NoError(t, f.DeleteSheet("Sheet1"))

	// Перемешанный заголовок: ФИО первым, Группа последней.
	shuffled := []string{"ФИО", "Дата", "Время", "Тип занятия", "Номер занятия",
		"Посещение", "Выполнено лаб", "Зачёт автоматом", "Подгруппа", "Группа"}
	data := []any{"Перем П", "23.09.2024", "08:00", "lab", 1, 1, 5, 0, 1, "101б"}
	for col, h := range shuffled {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		require.NoError(t, f.SetCellValue(attendance.SheetName, cell, h))
	}
	for col, v := range data {
		cell, _ := excelize.CoordinatesToCellName(col+1, 2)
		require.NoError(t, f.SetCellValue(attendance.SheetName, cell, v))
	}
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))

	rep, err := newSvc().Calculate(buf.Bytes())
	require.NoError(t, err)
	s := findStudent(t, rep, "101б", "Перем П")
	require.Equal(t, 5, s.SuccessLabs)
}

// --- Негативные тесты парсинга ---

// TestCalculate_SheetNotFound — нет листа «Посещаемость» → ErrSheetNotFound.
func TestCalculate_SheetNotFound(t *testing.T) {
	data := buildXLSX(t, "ДругойЛист", true, [][]any{
		row("101б", 1, "Икс И", "23.09.2024", "08:00", "lab", 1, 1, 5, 0),
	})
	_, err := newSvc().Calculate(data)
	require.ErrorIs(t, err, attendance.ErrSheetNotFound)
}

// TestCalculate_MissingColumns — нет обязательной колонки ФИО → ErrMissingColumns (TC-24).
func TestCalculate_MissingColumns(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	idx, _ := f.NewSheet(attendance.SheetName)
	f.SetActiveSheet(idx)
	require.NoError(t, f.DeleteSheet("Sheet1"))
	// Заголовок без «ФИО».
	noName := []string{"Группа", "Дата", "Время", "Тип занятия", "Номер занятия", "Посещение", "Выполнено лаб"}
	for col, h := range noName {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		require.NoError(t, f.SetCellValue(attendance.SheetName, cell, h))
	}
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))

	_, err := newSvc().Calculate(buf.Bytes())
	require.ErrorIs(t, err, attendance.ErrMissingColumns)
}

// TestCalculate_InvalidRows — некорректные значения в строках → ErrRowInvalid (TC-26/27).
func TestCalculate_InvalidRows(t *testing.T) {
	cases := []struct {
		name string
		bad  []any
	}{
		{"плохая дата", row("101б", 1, "Х Х", "23-09-2024", "08:00", "lab", 1, 1, 5, 0)},
		{"плохое время", row("101б", 1, "Х Х", "23.09.2024", "8.00", "lab", 1, 1, 5, 0)},
		{"плохой тип", row("101б", 1, "Х Х", "23.09.2024", "08:00", "семинар", 1, 1, 5, 0)},
		{"плохой флаг посещения", row("101б", 1, "Х Х", "23.09.2024", "08:00", "lab", 1, "может быть", 5, 0)},
		{"пустое ФИО", row("101б", 1, "", "23.09.2024", "08:00", "lab", 1, 1, 5, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := buildXLSX(t, attendance.SheetName, true, [][]any{tc.bad})
			_, err := newSvc().Calculate(data)
			require.ErrorIs(t, err, attendance.ErrRowInvalid)
		})
	}
}

// TestCalculate_FlagSynonyms — «да»/«нет» в колонке посещения принимаются (TC-27).
func TestCalculate_FlagSynonyms(t *testing.T) {
	rows := [][]any{
		row("101б", 1, "Да Д", "23.09.2024", "08:00", "lab", 1, "да", 5, "нет"),
	}
	rep, err := newSvc().Calculate(buildXLSX(t, attendance.SheetName, true, rows))
	require.NoError(t, err)
	s := findStudent(t, rep, "101б", "Да Д")
	require.Equal(t, 100, s.VisitPercent)
}

// TestCalculate_NotXLSX — мусорные байты вместо книги Excel → ошибка (не паника).
func TestCalculate_NotXLSX(t *testing.T) {
	_, err := newSvc().Calculate([]byte("это не xlsx, а просто текст"))
	require.Error(t, err)
	// Не структурная ошибка домена — это нечитаемый файл.
	require.NotErrorIs(t, err, attendance.ErrSheetNotFound)
	require.NotErrorIs(t, err, attendance.ErrMissingColumns)
}
