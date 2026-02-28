package controller

import (
	"encoding/json"
	"lab1/internal/dto"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockInfoService struct{}

func (m *mockInfoService) GetServerInfo() dto.ServerInfoDTO {
	return dto.ServerInfoDTO{
		GoVersion: "go1.99.mock",
		OS:        "mockOS",
		Arch:      "mockArch",
	}
}

func (m *mockInfoService) GetDatabaseInfo() dto.DatabaseInfoDTO {
	return dto.DatabaseInfoDTO{
		Driver:       "MockDB",
		Version:      "99.9",
		DatabaseName: "mock_database",
	}
}

// Тест 1: Проверка информации о сервере
func TestInfoController_ServerInfo(t *testing.T) {
	mockSvc := &mockInfoService{}
	ctrl := NewInfoController(mockSvc)

	req, err := http.NewRequest("GET", "/info/server", nil)
	if err != nil {
		t.Fatalf("Не удалось создать запрос: %v", err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ctrl.ServerInfo)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получено %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Неверный Content-Type")
	}

	var responseDTO dto.ServerInfoDTO
	if err := json.NewDecoder(rr.Body).Decode(&responseDTO); err != nil {
		t.Fatalf("Ошибка декодирования JSON: %v", err)
	}

	if responseDTO.GoVersion != "go1.99.mock" || responseDTO.OS != "mockOS" {
		t.Errorf("Получены неверные данные DTO: %+v", responseDTO)
	}
}

// Тест 2: Проверка информации о базе данных
func TestInfoController_DatabaseInfo(t *testing.T) {
	mockSvc := &mockInfoService{}
	ctrl := NewInfoController(mockSvc)

	req, err := http.NewRequest("GET", "/info/database", nil)
	if err != nil {
		t.Fatalf("Не удалось создать запрос: %v", err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ctrl.DatabaseInfo)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получено %d", rr.Code)
	}

	var responseDTO dto.DatabaseInfoDTO
	if err := json.NewDecoder(rr.Body).Decode(&responseDTO); err != nil {
		t.Fatalf("Ошибка декодирования JSON: %v", err)
	}

	if responseDTO.Driver != "MockDB" || responseDTO.Version != "99.9" {
		t.Errorf("Получены неверные данные БД: %+v", responseDTO)
	}
}

// Тест 3: Проверка информации о клиенте (Берет данные из Request, а не сервиса)
func TestInfoController_ClientInfo(t *testing.T) {
	mockSvc := &mockInfoService{}
	ctrl := NewInfoController(mockSvc)

	req, err := http.NewRequest("GET", "/info/client", nil)
	if err != nil {
		t.Fatalf("Не удалось создать запрос: %v", err)
	}

	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("User-Agent", "TestMockBrowser/1.0")

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(ctrl.ClientInfo)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получено %d", rr.Code)
	}

	var responseDTO dto.ClientInfoDTO
	if err := json.NewDecoder(rr.Body).Decode(&responseDTO); err != nil {
		t.Fatalf("Ошибка декодирования JSON: %v", err)
	}

	if responseDTO.IPAddress != "192.168.1.1:12345" || responseDTO.UserAgent != "TestMockBrowser/1.0" {
		t.Errorf("Получены неверные данные клиента: %+v", responseDTO)
	}
}
