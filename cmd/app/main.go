package main

import (
	"fmt"
	"log"
	"time"
	"yugu-server/internal/controller"
	"yugu-server/internal/router"
	"yugu-server/internal/service"
)

func main() {
	svc := service.NewInfoService()
	ctrl := controller.NewInfoController(svc)

	r := router.SetupRouter(ctrl)

	loc, _ := time.LoadLocation("Europe/Moscow")
	time.Local = loc

	fmt.Println("-----------------------------------------------")
	fmt.Printf("⏰ Таймзона установлена: %s\n", time.Local.String())
	fmt.Printf("🚀 Yugu Server запущен на http://127.0.0.1:8000\n")
	fmt.Println("-----------------------------------------------")

	if err := r.Run(":8000"); err != nil {
		log.Fatalf("Ошибка запуска: %v", err)
	}
}
