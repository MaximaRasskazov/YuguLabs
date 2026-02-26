package dto

// ServerInfoDTO содержит информацию о сервере
type ServerInfoDTO struct {
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// ClientInfoDTO содержит данные клиента
type ClientInfoDTO struct {
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
}

// DatabaseInfoDTO содержит информацию о БД
type DatabaseInfoDTO struct {
	Driver       string `json:"driver"`
	Version      string `json:"version"`
	DatabaseName string `json:"database_name"`
}
