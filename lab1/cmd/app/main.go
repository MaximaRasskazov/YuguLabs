package main

import (
	"fmt"
	"go-lab1/internal/controller"
	"go-lab1/internal/service"
	"log"
	"net/http"
	"time"
)

func main() {
	// 1. Конфигурация приложения: Установка временной зоны (по заданию)
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Fatalf("Ошибка загрузки временной зоны: %v", err)
	}
	time.Local = loc
	fmt.Printf("Установлена временная зона: %s\n", time.Local.String())

	// 2. Инициализация зависимостей (DI)
	infoService := service.NewInfoService()
	infoController := controller.NewInfoController(infoService)

	// 3. Настройка маршрутов (используем возможности Go 1.22+)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /info/server", infoController.ServerInfo)
	mux.HandleFunc("GET /info/client", infoController.ClientInfo)
	mux.HandleFunc("GET /info/database", infoController.DatabaseInfo)

	// 4. Запуск сервера
	port := ":8000"
	fmt.Printf("Сервер запущен на http://127.0.0.1%s\n", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}
