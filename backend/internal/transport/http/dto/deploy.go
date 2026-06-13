package dto

import "github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/deploy"

// DeployStep — одна выполненная git-команда в ответе webhook'а.
type DeployStep struct {
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
}

// DeployResponse — успешный JSON-ответ webhook авто-деплоя.
type DeployResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Branch  string `json:"branch"`
	// Warnings — нефатальные предупреждения (например, «грязное» рабочее
	// дерево, чьи правки отброшены). Пусто — поле опускается.
	Warnings []string     `json:"warnings,omitempty"`
	Steps    []DeployStep `json:"steps"`
}

// FromDeployResult маппит результат сервиса деплоя в JSON-ответ.
func FromDeployResult(r deploy.Result) DeployResponse {
	steps := make([]DeployStep, 0, len(r.Steps))
	for _, s := range r.Steps {
		steps = append(steps, DeployStep{Command: s.Command, Output: s.Output})
	}
	return DeployResponse{
		Status:   "success",
		Message:  "Deployment completed",
		Branch:   r.Branch,
		Warnings: r.Warnings,
		Steps:    steps,
	}
}
