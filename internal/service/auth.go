package service

import (
	"errors"
	"time"
	"log"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"yugu-server/internal/dto"
	"yugu-server/internal/repository"
)

type AuthService interface {
	Register(req dto.RegisterRequest) (dto.UserDTO, error)
	Login(req dto.LoginRequest, userAgent, ip string) (dto.AuthSuccessDTO, error)
	GetMe(userID uint) (dto.UserDTO, error)
	GetTokens(userID uint) ([]dto.SessionDTO, error)
	LogoutAll(userID uint) error 
	
	RefreshTokens(req dto.RefreshRequest, userAgent, ip string) (dto.AuthSuccessDTO, error)
	Logout(req dto.RefreshRequest) error
}

type authServiceImpl struct {
	db           *gorm.DB
	tokenService TokenService
}

func NewAuthService(db *gorm.DB, ts TokenService) AuthService {
	return &authServiceImpl{db: db, tokenService: ts}
}

func (s *authServiceImpl) Register(req dto.RegisterRequest) (dto.UserDTO, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return dto.UserDTO{}, err
	}

	birthday, _ := time.Parse("2006-01-02", req.Birthday)

	// модель для БД
	user := repository.User{
		Username: req.Username,
		Email:    req.Email,
		Password: string(hashedPassword),
		Birthday: birthday,
	}

	if err := s.db.Create(&user).Error; err != nil {
		log.Printf("Реальная ошибка: %v\n", err)
		return dto.UserDTO{}, errors.New("пользователь с таким логином или email уже существует")
	}

	return dto.UserDTO{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Birthday: user.Birthday.Format("2006-01-02"),
	}, nil
}

func (s *authServiceImpl) Login(req dto.LoginRequest, userAgent, ip string) (dto.AuthSuccessDTO, error) {
	var user repository.User

	if err := s.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		return dto.AuthSuccessDTO{}, errors.New("неверные учетные данные")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return dto.AuthSuccessDTO{}, errors.New("неверные учетные данные")
	}

	accessToken, refreshToken, err := s.tokenService.GenerateTokens(user.ID, userAgent, ip)
	if err != nil {
		return dto.AuthSuccessDTO{}, errors.New("ошибка при генерации токенов")
	}

	return dto.AuthSuccessDTO{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: dto.UserDTO{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
			Birthday: user.Birthday.Format("2006-01-02"),
		},
	}, nil
}

func (s *authServiceImpl) GetMe(userID uint) (dto.UserDTO, error) {
	var user repository.User
	
	if err := s.db.First(&user, userID).Error; err != nil {
		return dto.UserDTO{}, errors.New("пользователь не найден")
	}

	return dto.UserDTO{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Birthday: user.Birthday.Format("2006-01-02"),
	}, nil
}

func (s *authServiceImpl) GetTokens(userID uint) ([]dto.SessionDTO, error) {
	return s.tokenService.GetActiveSessions(userID)
}

func (s *authServiceImpl) LogoutAll(userID uint) error {
	return s.tokenService.RevokeAllSessions(userID)
}

func (s *authServiceImpl) RefreshTokens(req dto.RefreshRequest, userAgent, ip string) (dto.AuthSuccessDTO, error) {
	accessToken, newRefreshToken, err := s.tokenService.RefreshTokens(req.RefreshToken, userAgent, ip)
	if err != nil {
		return dto.AuthSuccessDTO{}, err
	}

	return dto.AuthSuccessDTO{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

func (s *authServiceImpl) Logout(req dto.RefreshRequest) error {
	return s.tokenService.RevokeSession(req.RefreshToken)
}