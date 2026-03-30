package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"yugu-server/internal/dto"

	"github.com/gin-gonic/gin"
)

// 1. Mock-сервис (оставляем как был)
type mockInfoService struct{}

func (m *mockInfoService) GetServerInfo() dto.ServerInfoDTO {
	return dto.ServerInfoDTO{GoVersion: "go1.99.mock", OS: "mockOS"}
}

func (m *mockInfoService) GetDatabaseInfo() dto.DatabaseInfoDTO {
	return dto.DatabaseInfoDTO{Driver: "MockDB", Version: "99.9"}
}

func (m *mockInfoService) GetClientInfo(ip, ua, lang string) dto.ClientInfoDTO {
	return dto.ClientInfoDTO{IPAddress: ip, UserAgent: ua, Language: lang}
}

func TestInfoController_ClientInfo(t *testing.T) {
	// Устанавливаем тестовый режим Gin, чтобы не спамил логами
	gin.SetMode(gin.TestMode)

	mockSvc := &mockInfoService{}
	ctrl := NewInfoController(mockSvc)

	// Настраиваем тестовый роутер
	r := gin.New()
	r.GET("/info/client", ctrl.ClientInfo)

	// Создаем фейковый запрос с нужными заголовками
	req, _ := http.NewRequest("GET", "/info/client", nil)
	req.Header.Set("User-Agent", "TestAgent")
	req.Header.Set("Accept-Language", "ru-RU")

	// Recorder для записи ответа
	w := httptest.NewRecorder()

	// Запуск
	r.ServeHTTP(w, req)

	// Проверки
	if w.Code != http.StatusOK {
		t.Errorf("Ожидался 200, получили %d", w.Code)
	}

	var res dto.ClientInfoDTO
	json.Unmarshal(w.Body.Bytes(), &res)

	if res.UserAgent != "TestAgent" || res.Language != "ru-RU" {
		t.Errorf("Данные в JSON не совпадают: %+v", res)
	}
}
