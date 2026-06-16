package attendance

import (
	"math"
	"sort"
)

// Calculator превращает плоский список строк файла в итоги по студентам и
// группам. Инкапсулирует все «правила зачёта» из ТЗ; не знает ни про HTTP,
// ни про Excel. Параметры зачёта берутся из конфигурации (внедряются в New).
type Calculator struct {
	requiredLabs int // сколько лаб нужно для зачёта
	threshold    int // минимальный процент посещаемости
}

// NewCalculator создаёт калькулятор с правилами зачёта.
func NewCalculator(requiredLabs, threshold int) *Calculator {
	return &Calculator{requiredLabs: requiredLabs, threshold: threshold}
}

// lessonKey — ключ уникального занятия: дата|время|тип|номер. Группа НЕ входит
// в ключ — занятие одно для всех групп/подгрупп (ТЗ, шаг 1).
type lessonKey struct {
	date, time, typ string
	number          int
}

func keyOf(r RawRow) lessonKey {
	return lessonKey{date: r.Date, time: r.Time, typ: r.Type, number: r.Number}
}

// studentKey — ключ студента: группа + ФИО. По ТЗ ФИО уникально в пределах
// группы, но группируем по паре для безопасности (однофамильцы в разных группах).
type studentKey struct {
	group, name string
}

// studentAgg — накопитель данных по одному студенту в процессе агрегации.
type studentAgg struct {
	group      string
	name       string
	subgroup   int
	doneLabs   int
	autoCredit bool
	// visited[lessonKey] = true, если студент посетил это занятие.
	visited map[lessonKey]bool
	// subByLesson — подгруппа студента на конкретном занятии (для поля subgroups).
	subByLesson map[lessonKey]int
}

// Calculate выполняет шаги 1–4 ТЗ и возвращает результаты, сгруппированные по
// группам (отсортированы по имени группы для детерминированного вывода).
//
// Один проход собирает: множество уникальных занятий (всего), агрегаты по
// студентам и итоги по группам — без повторных обходов и БД-запросов.
func (c *Calculator) Calculate(rows []RawRow) []GroupResult {
	// Шаг 1: множество всех уникальных занятий (общий знаменатель посещаемости).
	allLessons := make(map[lessonKey]struct{})
	// Шаг 2: агрегаты по студентам.
	students := make(map[studentKey]*studentAgg)

	for _, r := range rows {
		lk := keyOf(r)
		allLessons[lk] = struct{}{}

		sk := studentKey{group: r.Group, name: r.FullName}
		agg, ok := students[sk]
		if !ok {
			agg = &studentAgg{
				group:       r.Group,
				name:        r.FullName,
				subgroup:    r.Subgroup,
				visited:     make(map[lessonKey]bool),
				subByLesson: make(map[lessonKey]int),
			}
			students[sk] = agg
		}

		// Профиль студента: подгруппа из любой его строки (по ТЗ она одна).
		agg.subgroup = r.Subgroup
		// Количество сданных лаб — максимум по строкам студента: устойчиво к
		// расхождению значений в разных строках (TC-19), не падаем.
		if r.DoneLabs > agg.doneLabs {
			agg.doneLabs = r.DoneLabs
		}
		// Авто-зачёт = true, если стоит хотя бы в одной строке студента.
		if r.AutoCredit {
			agg.autoCredit = true
		}
		// Посещение: дубликаты строк одного занятия схлопываются (TC-28) —
		// карта по ключу занятия даёт идемпотентность.
		if r.Visited {
			agg.visited[lk] = true
		} else if _, seen := agg.visited[lk]; !seen {
			agg.visited[lk] = false
		}
		// Подгруппа студента на этом занятии (для поля subgroups в leasons).
		agg.subByLesson[lk] = r.Subgroup
	}

	totalLessons := len(allLessons)

	// Шаги 3–4: считаем результат каждого студента и группируем.
	byGroup := make(map[string][]StudentResult)
	for _, agg := range students {
		byGroup[agg.group] = append(byGroup[agg.group], c.studentResult(agg, totalLessons))
	}

	return assembleGroups(byGroup)
}

// studentResult вычисляет итог одного студента (шаг 3 ТЗ).
func (c *Calculator) studentResult(agg *studentAgg, totalLessons int) StudentResult {
	lessons := buildLessons(agg)

	visitedCount := 0
	for _, v := range agg.visited {
		if v {
			visitedCount++
		}
	}

	visitPercent := percent(visitedCount, totalLessons)
	successLabsPercent := percent(agg.doneLabs, c.requiredLabs)

	// Зачёт: авто-зачёт ИЛИ (посещаемость ≥ порог И лаб ≥ нужного минимума).
	result := agg.autoCredit ||
		(visitPercent >= c.threshold && agg.doneLabs >= c.requiredLabs)

	return StudentResult{
		Name:               agg.name,
		Subgroup:           agg.subgroup,
		Lessons:            lessons,
		VisitPercent:       visitPercent,
		SuccessLabsPercent: successLabsPercent,
		SuccessLabs:        agg.doneLabs,
		Result:             result,
	}
}

// buildLessons собирает срез занятий студента, отсортированный по дате и
// времени (по возрастанию) — требование ТЗ к массиву leasons.
func buildLessons(agg *studentAgg) []Lesson {
	lessons := make([]Lesson, 0, len(agg.visited))
	for lk, visited := range agg.visited {
		lessons = append(lessons, Lesson{
			Date:     lk.date,
			Time:     lk.time,
			Type:     lk.typ,
			Number:   lk.number,
			Subgroup: agg.subByLesson[lk],
			Visit:    visited,
		})
	}
	sort.Slice(lessons, func(i, j int) bool {
		di, dj := dateSortKey(lessons[i].Date), dateSortKey(lessons[j].Date)
		if di != dj {
			return di < dj
		}
		return lessons[i].Time < lessons[j].Time
	})
	return lessons
}

// assembleGroups превращает map[group][]student в отсортированный по имени
// группы срез GroupResult со счётчиками успешных/неуспешных (шаг 4 ТЗ).
func assembleGroups(byGroup map[string][]StudentResult) []GroupResult {
	groups := make([]GroupResult, 0, len(byGroup))
	for name, studs := range byGroup {
		// Студенты внутри группы — по ФИО, для стабильного вывода.
		sort.Slice(studs, func(i, j int) bool { return studs[i].Name < studs[j].Name })

		var success, fail int
		for _, s := range studs {
			if s.Result {
				success++
			} else {
				fail++
			}
		}
		groups = append(groups, GroupResult{
			GroupName: name,
			Students:  studs,
			Result:    GroupCounters{Success: success, Unsuccessfully: fail},
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].GroupName < groups[j].GroupName })
	return groups
}

// percent считает round(part/total*100). Делитель 0 → 0% (пустой файл не
// должен вызывать деление на ноль).
func percent(part, total int) int {
	if total <= 0 {
		return 0
	}
	return int(math.Round(float64(part) / float64(total) * 100))
}

// dateSortKey превращает «ДД.ММ.ГГГГ» в сортируемое «ГГГГММДД». На вход всегда
// уже валидная (проверенная парсером) дата, поэтому без обработки ошибок:
// при неожиданном формате вернём исходную строку — порядок не сломается фатально.
func dateSortKey(d string) string {
	// d == "ДД.ММ.ГГГГ"
	if len(d) != 10 || d[2] != '.' || d[5] != '.' {
		return d
	}
	return d[6:10] + d[3:5] + d[0:2]
}
