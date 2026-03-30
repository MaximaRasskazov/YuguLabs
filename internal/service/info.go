package service

import (
	"runtime"
	"yugu-server/internal/dto"

	"gorm.io/gorm"
)

type InfoService interface {
	GetServerInfo() dto.ServerInfoDTO
	GetDatabaseInfo() dto.DatabaseInfoDTO
	GetClientInfo(ip, userAgent, lang string) dto.ClientInfoDTO
}

type infoServiceImpl struct {
	db *gorm.DB
}

func NewInfoService(db *gorm.DB) InfoService {
	return &infoServiceImpl{db: db}
}

func (s *infoServiceImpl) GetServerInfo() dto.ServerInfoDTO {
	return dto.ServerInfoDTO{
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func (s *infoServiceImpl) GetDatabaseInfo() dto.DatabaseInfoDTO {
	var version string

	if s.db != nil {
		s.db.Raw("SELECT sqlite_version()").Scan(&version)
	}

	if version == "" {
		version = "unknown"
	}

	return dto.DatabaseInfoDTO{
		Driver:       "SQLite3",
		Version:      version,
		DatabaseName: "app.db",
	}
}

func (s *infoServiceImpl) GetClientInfo(ip, userAgent, lang string) dto.ClientInfoDTO {
	return dto.ClientInfoDTO{
		IPAddress: ip,
		UserAgent: userAgent,
		Language:  lang,
	}
}
