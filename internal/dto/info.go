package dto

type ServerInfoDTO struct {
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

type ClientInfoDTO struct {
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
	Language  string `json:"language"`
}

type DatabaseInfoDTO struct {
	Driver       string `json:"driver"`
	Version      string `json:"version"`
	DatabaseName string `json:"database_name"`
}

// RoleDTO для безопасной отдачи ролей (без служебных полей created_by и т.д.)
type RoleDTO struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"` // omitempty скроет поле, если оно null
}

// PermissionDTO для безопасной отдачи разрешений
type PermissionDTO struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
}
