package http

import (
	"testing"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/config"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/rbac"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/token"
)

// TestNewRouter_NoRouteConflicts строит роутер и проверяет, что chi не
// падает на конфликте паттернов под /api/users/{id}/* (avatar/roles/story
// зарегистрированы из разных mount-функций). Регистрация маршрутов не
// требует живых зависимостей — методы сервисов на этом этапе не вызываются.
func TestNewRouter_NoRouteConflicts(t *testing.T) {
	_ = NewRouter(Deps{
		Cfg:       &config.Config{},
		RBAC:      &rbac.Service{},
		Tokens:    &token.Service{},
		Changelog: &changelog.Service{},
	})
}
