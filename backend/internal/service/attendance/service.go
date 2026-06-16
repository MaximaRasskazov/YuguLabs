package attendance

// Service — фасад лабы №12: связывает Parser, Calculator и ResponseBuilder в
// один вызов Calculate(content). Это composition root домена — handler знает
// только про Service, а не про три внутренних класса.
type Service struct {
	parser  *Parser
	calc    *Calculator
	builder *ResponseBuilder
}

// New собирает Service с правилами зачёта (из config: REQUIRED_LABS,
// ATTENDANCE_PERCENT_THRESHOLD).
func New(requiredLabs, attendanceThreshold int) *Service {
	return &Service{
		parser:  NewParser(),
		calc:    NewCalculator(requiredLabs, attendanceThreshold),
		builder: NewResponseBuilder(),
	}
}

// Calculate выполняет полный конвейер: разбор .xlsx → расчёт → сборка ответа.
// Ошибка парсинга (ErrSheetNotFound / ErrMissingColumns / ErrRowInvalid)
// пробрасывается наружу — handler маппит её в 422.
func (s *Service) Calculate(content []byte) (Report, error) {
	rows, err := s.parser.Parse(content)
	if err != nil {
		return Report{}, err
	}
	groups := s.calc.Calculate(rows)
	return s.builder.Build(groups), nil
}
