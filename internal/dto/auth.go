package dto

// Входящие запросы
type RegisterRequest struct {
	Username  string `json:"username" binding:"required,min=7,alpha_capital"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=8,password_complex"`
	CPassword string `json:"c_password" binding:"required,eqfield=Password"`
	Birthday  string `json:"birthday" binding:"required,datetime=2006-01-02,age_14"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required,min=7,alpha_capital"`
	Password string `json:"password" binding:"required,min=8"`
}

// Исходящие объекты
type UserDTO struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Birthday string `json:"birthday"`
}

type AuthSuccessDTO struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	User         UserDTO `json:"user"`
}

type TokenListDTO struct {
	ID        uint   `json:"id"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	UserAgent string `json:"user_agent"`
	IPAddress string `json:"ip_address"`
}

type SessionDTO struct {
	ID        uint   `json:"id"`
	UserAgent string `json:"user_agent"`
	IPAddress string `json:"ip_address"`
	ExpiresAt string `json:"expires_at"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}