package controller

import (
	"fmt"
	"lab1/internal/service"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInfoController_AllRoutes(t *testing.T) {
	svc := service.NewInfoService()
	ctrl := NewInfoController(svc)

	tests := []struct {
		name    string
		route   string
		handler http.HandlerFunc
	}{
		{"Server Info", "/info/server", ctrl.ServerInfo},
		{"Client Info", "/info/client", ctrl.ClientInfo},
		{"Database Info", "/info/database", ctrl.DatabaseInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.route, nil)
			rr := httptest.NewRecorder()

			tt.handler.ServeHTTP(rr, req)

			if rr.Code == http.StatusOK {
				fmt.Printf("✅ %s: Статус 200 OK\n", tt.name)
			}

			contentType := rr.Header().Get("Content-Type")
			if contentType == "application/json" {
				fmt.Printf("✅ %s: Content-Type JSON подтвержден\n\n", tt.name)
			}
		})
	}
}
