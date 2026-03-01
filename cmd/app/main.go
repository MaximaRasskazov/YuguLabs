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
	// 1. Инициализация слоев
	svc := service.NewInfoService()
	ctrl := controller.NewInfoController(svc)

	// 2. Настройка роутера (Здесь Gin выведет свои debug-сообщения)
	r := router.SetupRouter(ctrl)

	// 3. Настройка таймзоны
	loc, _ := time.LoadLocation("Europe/Moscow")
	time.Local = loc

	// 4. Твой кастомный вывод (теперь он будет в самом низу)
	fmt.Println("-----------------------------------------------")
	fmt.Printf("⏰ Таймзона установлена: %s\n", time.Local.String())
	fmt.Printf("🚀 Yugu Server запущен на http://127.0.0.1:8000\n")
	fmt.Println("-----------------------------------------------")

	// 5. Запуск (Gin начнет слушать порт и больше не будет спамить до прихода запроса)
	if err := r.Run(":8000"); err != nil {
		log.Fatalf("Ошибка запуска: %v", err)
	}
}
