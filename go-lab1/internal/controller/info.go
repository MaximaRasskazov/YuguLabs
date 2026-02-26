package controller

import (
	"encoding/json"
	"net/http"

	"go-lab1/internal/dto"
	"go-lab1/internal/service"
)

type InfoController struct {
	service service.InfoService // Инъекция зависимости (Dependency Inversion)
}

func NewInfoController(s service.InfoService) *InfoController {
	return &InfoController{service: s}
}

// writeJSON - вспомогательный метод для отправки DTO
func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *InfoController) ServerInfo(w http.ResponseWriter, r *http.Request) {
	data := c.service.GetServerInfo()
	writeJSON(w, data)
}

func (c *InfoController) ClientInfo(w http.ResponseWriter, r *http.Request) {
	// IP и User-Agent берем прямо из запроса, сервис тут не нужен
	data := dto.ClientInfoDTO{
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	writeJSON(w, data)
}

func (c *InfoController) DatabaseInfo(w http.ResponseWriter, r *http.Request) {
	data := c.service.GetDatabaseInfo()
	writeJSON(w, data)
}