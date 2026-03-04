## 🚀 Как запустить сервер

1. Запуск приложения:
    ```bash
    go run cmd/app/main.go
    ```
2. Провекрка эндпоинтов:
    ```bash
    curl http://127.0.0.1:8000/info/server
    curl http://127.0.0.1:8000/info/client
    curl http://127.0.0.1:8000/info/database
    ```
3. Авто-Тестирование:
    ```bash
    go test ./internal/controller/... -v
    ```
## 📂 Структура проекта

```text
go-lab1/
├── cmd/
│   └── app/
│       └── main.go           # Точка входа
├── internal/
│   ├── controller/
│   │   ├── info.go           # Хендлеры системной информации
│   │   └── auth.go           # ЛАБА_2: Хендлеры авторизации (login, register и т.д.)
│   ├── dto/
│   │   ├── info.go           # DTO системной информации
│   │   └── auth.go           # ЛАБА_2: UserDTO, AuthSuccessDTO, LoginRequest и т.д.
│   ├── middleware/
│   │   └── auth.go           # ЛАБА_2: Проверка токена доступа для защищенных маршрутов
│   ├── repository/
│   │   └── sqlite.go         # ЛАБА_2: Подключение к SQLite и работа с БД
│   └── service/
│       ├── info.go           # Логика системной информации
│       ├── auth.go           # ЛАБА_2: Логика регистрации и входа
│       └── token.go          # ЛАБА_2: Генерация, хеширование и отзыв токенов
├── database/
│   └── app.db                # ЛАБА_2: Файл базы данных SQLite
├── .env                      # Настройки (включая ACCESS_TOKEN_TTL)
├── .gitignore                    
└── go.mod
```
