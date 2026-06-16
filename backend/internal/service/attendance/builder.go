package attendance

import "sort"

// ResponseBuilder собирает наружный ответ из посчитанных групп (шаг 5 ТЗ):
// объединяет группы и дополнительный плоский список зачётников
// (automatic_success_students). Вынесен отдельным классом по SRP, чтобы
// форму ответа можно было менять, не трогая расчёт.
type ResponseBuilder struct{}

// NewResponseBuilder создаёт построитель ответа.
func NewResponseBuilder() *ResponseBuilder { return &ResponseBuilder{} }

// Build формирует Report: те же группы плюс сквозной список всех студентов с
// result=true (group+ФИО), отсортированный по группе и ФИО для стабильности.
func (b *ResponseBuilder) Build(groups []GroupResult) Report {
	successList := make([]SuccessStudent, 0)
	for _, g := range groups {
		for _, s := range g.Students {
			if s.Result {
				successList = append(successList, SuccessStudent{Group: g.GroupName, Name: s.Name})
			}
		}
	}
	sort.Slice(successList, func(i, j int) bool {
		if successList[i].Group != successList[j].Group {
			return successList[i].Group < successList[j].Group
		}
		return successList[i].Name < successList[j].Name
	})

	return Report{
		Groups:                   groups,
		AutomaticSuccessStudents: successList,
	}
}
