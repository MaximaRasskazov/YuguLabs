package service

import (
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"yugu-server/internal/dto"
	"yugu-server/internal/repository"
)

type AuthService interface {
	Register(req dto.RegisterRequest) (dto.UserDTO, error)
	Login(req dto.LoginRequest, userAgent, ip string) (dto.AuthSuccessDTO, error)
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