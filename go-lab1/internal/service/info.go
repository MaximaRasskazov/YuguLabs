package service

import (
	"go-lab1/internal/dto"
	"runtime"
)

// InfoService определяет контракт для получения информации
type InfoService interface {
	GetServerInfo() dto.ServerInfoDTO
	GetDatabaseInfo() dto.DatabaseInfoDTO
}

// infoServiceImpl - конкретная реализация сервиса
type infoServiceImpl struct {
	// Здесь мог бы быть пул соединений с БД (sql.DB)
}

func NewInfoService() InfoService {
	return &infoServiceImpl{}
}

func (s *infoServiceImpl) GetServerInfo() dto.ServerInfoDTO {
	// Собираем данные интерпретатора (в нашем случае - рантайма Go)
	return dto.ServerInfoDTO{
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func (s *infoServiceImpl) GetDatabaseInfo() dto.DatabaseInfoDTO {
	// Имитация похода в базу данных.
	// В реальном проекте здесь был бы SQL-запрос `SELECT version();`
	return dto.DatabaseInfoDTO{
		Driver:       "PostgreSQL",
		Version:      "15.4",
		DatabaseName: "lab1_db",
	}
}
