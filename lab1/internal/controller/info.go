package controller

import (
	"encoding/json"
	"lab1/internal/dto"
	"lab1/internal/service"
	"net/http"
)

type InfoController struct {
	service service.InfoService
}

func NewInfoController(s service.InfoService) *InfoController {
	return &InfoController{service: s}
}

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
