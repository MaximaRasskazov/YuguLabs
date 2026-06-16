package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/attendance"
)

// AttendanceHandler обслуживает POST /api/attendance/calculate (лаба №12):
// принимает .xlsx с посещаемостью, считает авто-зачёт и возвращает JSON.
type AttendanceHandler struct {
	svc         *attendance.Service
	maxUploadMB int
}

// NewAttendanceHandler собирает обработчик. maxUploadMB — потолок размера
// файла из конфигурации (UPLOAD_MAX_SIZE_MB).
func NewAttendanceHandler(svc *attendance.Service, maxUploadMB int) *AttendanceHandler {
	return &AttendanceHandler{svc: svc, maxUploadMB: maxUploadMB}
}

// Calculate godoc
//
//	@Summary		Авто-расчёт зачёта по файлу успеваемости
//	@Description	Принимает .xlsx (лист «Посещаемость»), считает процент посещаемости и сданных лаб, определяет студентов с авто-зачётом. Требует право calculate-attendance (только админ).
//	@Tags			attendance
//	@Accept			mpfd
//	@Produce		json
//	@Param			file	formData	file	true	"Excel-файл .xlsx с посещаемостью"
//	@Success		200		{object}	attendance.Report
//	@Failure		400		{object}	dto.ErrorResponse	"Файл не приложен/повреждён"
//	@Failure		413		{object}	dto.ErrorResponse	"Файл больше UPLOAD_MAX_SIZE_MB"
//	@Failure		422		{object}	dto.ErrorResponse	"Некорректная структура/данные файла"
//	@Security		BearerAuth
//	@Router			/api/attendance/calculate [post]
func (h *AttendanceHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	// Жёсткий потолок тела: UPLOAD_MAX_SIZE_MB (+1КБ на multipart-обвязку),
	// чтобы не читать в память заведомо огромный аплоад.
	limit := int64(h.maxUploadMB)*1024*1024 + 1<<10
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(limit); err != nil {
		if isMaxBytes(err) {
			h.tooLarge(w)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_body",
			"Не удалось разобрать форму загрузки. Отправьте файл как multipart/form-data в поле «file».")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_file",
			"Файл не найден в запросе. Приложите .xlsx в поле «file».")
		return
	}
	defer func() { _ = file.Close() }()

	// Расширение файла намеренно не проверяется: проверка по сигнатуре
	// (первые 4 байта ZIP PK\x03\x04) происходит в parser.Parse —
	// она надёжнее, так как не зависит от имени файла.
	content, err := io.ReadAll(file)
	if err != nil {
		if isMaxBytes(err) {
			h.tooLarge(w)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_file",
			"Не удалось прочитать содержимое файла. Попробуйте загрузить его ещё раз.")
		return
	}

	report, err := h.svc.Calculate(content)
	if err != nil {
		h.mapCalcError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// tooLarge — единый ответ 413 с актуальным лимитом из конфигурации.
func (h *AttendanceHandler) tooLarge(w http.ResponseWriter) {
	writeError(w, http.StatusRequestEntityTooLarge, "file_too_large",
		"Файл больше допустимого размера.")
}

// mapCalcError маппит ошибки парсинга в 422 (некорректный файл) либо в 400
// (нечитаемый .xlsx). Содержательное сообщение помогает понять, что не так —
// особенно для битой строки (ErrRowInvalid уже несёт номер строки и причину).
func (h *AttendanceHandler) mapCalcError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, attendance.ErrNotXLSX):
		writeError(w, http.StatusUnprocessableEntity, "invalid_file_type",
			"Файл не является XLSX — проверьте формат (ожидается книга Excel .xlsx).")
	case errors.Is(err, attendance.ErrSheetNotFound):
		writeError(w, http.StatusUnprocessableEntity, "sheet_not_found",
			"В книге нет листа «Посещаемость» — проверьте имя листа.")
	case errors.Is(err, attendance.ErrMissingColumns):
		writeError(w, http.StatusUnprocessableEntity, "missing_columns", err.Error())
	case errors.Is(err, attendance.ErrTooManyRows):
		// Слишком много строк — защита от «xlsx-бомбы». 413: проблема в
		// размере полезной нагрузки, а не в её формате.
		writeError(w, http.StatusRequestEntityTooLarge, "too_many_rows",
			"В файле слишком много строк — превышен лимит обработки.")
	case errors.Is(err, attendance.ErrRowInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_row", err.Error())
	default:
		// Не распознали как .xlsx (битый/не тот формат) — это проблема запроса.
		writeError(w, http.StatusBadRequest, "invalid_file",
			"Не удалось обработать файл как .xlsx. Проверьте, что это корректная книга Excel.")
	}
}
