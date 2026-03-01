package main

import (
	"fmt"
	"lab1/internal/controller"
	"lab1/internal/service"
	"log"
	"net/http"
	"time"
)

func main() {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.Fatalf("Ошибка загрузки временной зоны: %v", err)
	}
	time.Local = loc
	fmt.Printf("Установлена временная зона: %s\n", time.Local.String())

	infoService := service.NewInfoService()
	infoController := controller.NewInfoController(infoService)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /info/server", infoController.ServerInfo)
	mux.HandleFunc("GET /info/client", infoController.ClientInfo)
	mux.HandleFunc("GET /info/database", infoController.DatabaseInfo)

	port := ":8000"
	fmt.Printf("Сервер запущен на http://127.0.0.1%s\n", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}
