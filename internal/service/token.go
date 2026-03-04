package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
	"yugu-server/internal/repository"
)

type TokenService interface {
	GenerateTokens(userID uint, userAgent, ip string) (string, string, error)
}

type tokenServiceImpl struct {
	db *gorm.DB
}

func NewTokenService(db *gorm.DB) TokenService {
	return &tokenServiceImpl{db: db}
}

func (s *tokenServiceImpl) GenerateTokens(userID uint, userAgent, ip string) (string, string, error) {
	accessTTL, _ := strconv.Atoi(getEnv("ACCESS_TOKEN_TTL", "60"))
	refreshTTL, _ := strconv.Atoi(getEnv("REFRESH_TOKEN_TTL", "10080"))
	maxTokens, _ := strconv.Atoi(getEnv("MAX_ACTIVE_TOKENS", "5"))
	secretKey := []byte(getEnv("JWT_SECRET", "default_secret"))

	// Access Token
	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(time.Duration(accessTTL) * time.Minute).Unix(),
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secretKey)
	if err != nil {
		return "", "", err
	}

	// Refresh Token
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	refreshToken := hex.EncodeToString(randomBytes)

	hash := sha256.Sum256([]byte(refreshToken))
	tokenHash := hex.EncodeToString(hash[:])

	var count int64
	s.db.Model(&repository.TokenSession{}).Where("user_id = ?", userID).Count(&count)

	if count >= int64(maxTokens) {
		var oldestToken repository.TokenSession
		s.db.Where("user_id = ?", userID).Order("created_at asc").First(&oldestToken)
		s.db.Delete(&oldestToken)
	}

	session := repository.TokenSession{
		UserID:    userID,
		TokenHash: tokenHash,
		UserAgent: userAgent,
		IPAddress: ip,
		ExpiresAt: time.Now().Add(time.Duration(refreshTTL) * time.Minute),
	}
	
	if err := s.db.Create(&session).Error; err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}