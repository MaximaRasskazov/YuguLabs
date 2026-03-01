package service

import (
	"runtime"
	"yugu-server/internal/dto"
)

type InfoService interface {
	GetServerInfo() dto.ServerInfoDTO
	GetDatabaseInfo() dto.DatabaseInfoDTO
	GetClientInfo(ip, userAgent, lang string) dto.ClientInfoDTO
}

type infoServiceImpl struct {
	// Добавить пул соединений с БД (sql.DB)
}

func NewInfoService() InfoService {
	return &infoServiceImpl{}
}

func (s *infoServiceImpl) GetServerInfo() dto.ServerInfoDTO {
	return dto.ServerInfoDTO{
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func (s *infoServiceImpl) GetDatabaseInfo() dto.DatabaseInfoDTO {
	return dto.DatabaseInfoDTO{
		Driver:       "PostgreSQL",
		Version:      "15.4",
		DatabaseName: "lab1_db",
	}
}

func (s *infoServiceImpl) GetClientInfo(ip, userAgent, lang string) dto.ClientInfoDTO {
	return dto.ClientInfoDTO{
		IPAddress: ip,
		UserAgent: userAgent,
		Language:  lang,
	}
}