## Структура проекта

```text
go-lab1/
├── cmd/
│   └── app/
│       └── main.go               # Точка входа, конфигурация (локаль, таймзона)
├── internal/
│   ├── controller/
│   │   └── info.go               # HTTP хендлеры
│   ├── dto/
│   │   └── info.go               # Data Transfer Objects
│   └── service/
│       └── info.go               # Бизнес-логика и работа с БД (или её имитация)
├── .env                           # Переменные окружения
├── .gitignore                     # Исключённые файлы
└── go.mod                         # Модуль Go
```
