package attendance

// White-box тест: проверяет защиту от «xlsx-бомбы», временно понижая лимит
// строк maxDataRows. Лежит в пакете attendance (не _test), чтобы иметь доступ
// к внутренней переменной — генерировать реальные 200k строк ради одного теста
// было бы слишком медленно.

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestParser_TooManyRows(t *testing.T) {
	// Понижаем лимит до 3 строк данных на время теста.
	orig := maxDataRows
	maxDataRows = 3
	defer func() { maxDataRows = orig }()

	header := []string{"Группа", "Подгруппа", "ФИО", "Дата", "Время",
		"Тип занятия", "Номер занятия", "Посещение", "Выполнено лаб", "Зачёт автоматом"}

	build := func(dataRows int) []byte {
		f := excelize.NewFile()
		defer func() { _ = f.Close() }()
		idx, err := f.NewSheet(SheetName)
		require.NoError(t, err)
		f.SetActiveSheet(idx)
		require.NoError(t, f.DeleteSheet("Sheet1"))
		for c, h := range header {
			cell, _ := excelize.CoordinatesToCellName(c+1, 1)
			require.NoError(t, f.SetCellValue(SheetName, cell, h))
		}
		for r := 0; r < dataRows; r++ {
			vals := []any{"101б", 1, "Студент", "23.09.2024", "08:00", "lab", r + 1, 1, 5, 0}
			for c, v := range vals {
				cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
				require.NoError(t, f.SetCellValue(SheetName, cell, v))
			}
		}
		var buf bytes.Buffer
		require.NoError(t, f.Write(&buf))
		return buf.Bytes()
	}

	p := NewParser()

	// Ровно на лимите (3 строки) — проходит.
	_, err := p.Parse(build(3))
	require.NoError(t, err)

	// На одну больше лимита (4 строки) — отсекается.
	_, err = p.Parse(build(4))
	require.ErrorIs(t, err, ErrTooManyRows)
}
