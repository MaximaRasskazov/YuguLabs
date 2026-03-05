package main

import (
	"fmt"
	"log"
	"time"

	"yugu-server/internal/controller"
	"yugu-server/internal/repository"
	"yugu-server/internal/router"
	"yugu-server/internal/service"
	customValidator "yugu-server/internal/validator" 

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func main() {
	if err := godotenv.Load(); err != nil {
        log.Println("Файл .env не найден, используются переменные окружения по умолчанию")
    }
	
	db := repository.SetupDatabase()

	// Регистрация кастомных валидаторов для Gin
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterValidation("alpha_capital", customValidator.ValidateUsername)
		v.RegisterValidation("password_complex", customValidator.ValidatePassword)
		v.RegisterValidation("age_14", customValidator.ValidateAge14)
	}

	infoSvc := service.NewInfoService()
	infoCtrl := controller.NewInfoController(infoSvc)

	tokenSvc := service.NewTokenService(db)
	authSvc := service.NewAuthService(db, tokenSvc)
	authCtrl := controller.NewAuthController(authSvc)

	r := router.SetupRouter(infoCtrl, authCtrl)

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