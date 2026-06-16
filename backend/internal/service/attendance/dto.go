// Package attendance реализует авто-зачёт по файлу успеваемости (лаба №12):
// разбор .xlsx с посещаемостью, подсчёт процентов посещения и сданных лаб,
// определение студентов, получающих зачёт автоматически, и сборка итогового
// JSON, сгруппированного по учебным группам.
//
// Логика разнесена по SRP на три класса (как требует ТЗ):
//   - Parser  (parser.go)     — чтение .xlsx и нормализация строк в []RawRow;
//   - Calculator (calculator.go) — агрегация по студентам и расчёт результата;
//   - ResponseBuilder (builder.go) — сборка наружного JSON ([]GroupResult).
//
// Service (service.go) — фасад, который связывает их в один вызов Calculate.
// Пакет не зависит от БД: на вход байты файла, на выход — структуры ответа.
package attendance

// SheetName — имя листа в книге Excel, откуда читаются данные (по ТЗ).
const SheetName = "Посещаемость"

// Заголовки колонок входного файла (первая строка листа). Порядок не важен —
// Parser ищет колонки по имени, а не по позиции, поэтому добавление/перестановка
// служебных колонок файл не ломает.
const (
	colGroup      = "Группа"
	colSubgroup   = "Подгруппа"
	colFullName   = "ФИО"
	colDate       = "Дата"
	colTime       = "Время"
	colType       = "Тип занятия"
	colNumber     = "Номер занятия"
	colVisit      = "Посещение"
	colDoneLabs   = "Выполнено лаб"
	colAutoCredit = "Зачёт автоматом"
)

// requiredColumns — колонки, без которых файл считается структурно некорректным
// (отвечаем 422). «Подгруппа» и «Зачёт автоматом» необязательны: пустая
// подгруппа трактуется как 1, отсутствующий авто-зачёт — как 0.
var requiredColumns = []string{
	colGroup, colFullName, colDate, colTime, colType, colNumber, colVisit, colDoneLabs,
}

// RawRow — одна нормализованная строка файла (DTO строки из ТЗ). Это уже
// разобранные и приведённые к типам данные одной записи «студент × занятие».
type RawRow struct {
	Group      string // номер группы, напр. "101б"
	Subgroup   int    // подгруппа студента; пустая в файле → 1
	FullName   string // ФИО студента
	Date       string // дата занятия в исходном формате ДД.ММ.ГГГГ
	Time       string // время начала ЧЧ:ММ
	Type       string // тип занятия: "lect" | "lab"
	Number     int    // номер лекции/лабораторной
	Visited    bool   // отметка о посещении (1 → true)
	DoneLabs   int    // сколько лаб выполнено (из колонки)
	AutoCredit bool   // «зачёт автоматом» (1 → true)
	// rowNum — номер строки в книге (1-based, с учётом заголовка) для
	// понятных сообщений об ошибках парсинга.
	rowNum int
}

// Lesson — уникальное занятие (DTO занятия из ТЗ). Идентифицируется ключом
// date|time|type|number — группа в ключ НЕ входит: одно и то же занятие у
// разных групп считается одним.
type Lesson struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	Type     string `json:"type"`
	Number   int    `json:"number"`
	Subgroup int    `json:"subgroups"` // подгруппа КОНКРЕТНОГО студента на этом занятии
	Visit    bool   `json:"visit"`     // посетил ли студент это занятие
}

// StudentResult — итог по одному студенту (DTO студента из ТЗ).
type StudentResult struct {
	Name               string   `json:"name"`
	Subgroup           int      `json:"subgroup"`
	Lessons            []Lesson `json:"leasons"` // орфография поля — из ТЗ (leasons)
	VisitPercent       int      `json:"visit_percent"`
	SuccessLabsPercent int      `json:"success_labs_percent"`
	SuccessLabs        int      `json:"success_labs"`
	Result             bool     `json:"result"`
}

// GroupCounters — агрегаты по группе: сколько студентов получили/не получили зачёт.
type GroupCounters struct {
	Success        int `json:"success"`
	Unsuccessfully int `json:"unsuccessfully"`
}

// GroupResult — итог по одной группе (DTO группы из ТЗ): список студентов и счётчики.
type GroupResult struct {
	GroupName string          `json:"group_name"`
	Students  []StudentResult `json:"students"`
	Result    GroupCounters   `json:"result"`
}

// SuccessStudent — запись доп.списка студентов, получивших зачёт.
type SuccessStudent struct {
	Group string `json:"group"`
	Name  string `json:"name"`
}

// Report — полный ответ метода Calculate: группы + плоский список зачётников.
type Report struct {
	Groups                   []GroupResult    `json:"groups"`
	AutomaticSuccessStudents []SuccessStudent `json:"automatic_success_students"`
}
