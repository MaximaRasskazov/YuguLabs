package service

import (
	"errors"
	"yugu-server/internal/dto"
	"yugu-server/internal/repository"

	"gorm.io/gorm"
)

type UserRoleService interface {
	AssignRole(targetUserID uint, roleID uint, currentUserID uint) error
	GetUserRoles(targetUserID uint) ([]dto.RoleDTO, error)
	HardRemoveRole(targetUserID uint, roleID uint) error
	SoftRemoveRole(targetUserID uint, roleID uint, currentUserID uint) error
	RestoreUserRole(targetUserID uint, roleID uint) error
}

type userRoleServiceImpl struct {
	db *gorm.DB
}

func NewUserRoleService(db *gorm.DB) UserRoleService {
	return &userRoleServiceImpl{db: db}
}

// AssignRole привязывает роль к пользователю
func (s *userRoleServiceImpl) AssignRole(targetUserID uint, roleID uint, currentUserID uint) error {
	// 1. Проверяем, существует ли пользователь и роль
	var userCount, roleCount int64
	s.db.Model(&repository.User{}).Where("id = ?", targetUserID).Count(&userCount)
	s.db.Model(&repository.Role{}).Where("id = ?", roleID).Count(&roleCount)

	if userCount == 0 {
		return errors.New("пользователь не найден")
	}
	if roleCount == 0 {
		return errors.New("роль не найдена")
	}

	// 2. Проверяем, нет ли уже такой активной связи (защита от дубликатов)
	var linkCount int64
	s.db.Model(&repository.UserRole{}).
		Where("user_id = ? AND role_id = ? AND deleted_at IS NULL", targetUserID, roleID).
		Count(&linkCount)

	if linkCount > 0 {
		return errors.New("пользователь уже имеет эту роль")
	}

	// 3. Создаем связь с указанием, КТО её создал (требование ТЗ)
	userRole := repository.UserRole{
		UserID:      targetUserID,
		RoleID:      roleID,
		CreatedByID: currentUserID,
	}

	return s.db.Create(&userRole).Error
}

// GetUserRoles возвращает список активных ролей конкретного пользователя
func (s *userRoleServiceImpl) GetUserRoles(targetUserID uint) ([]dto.RoleDTO, error) {
	var user repository.User

	// Используем Preload для автоматического JOIN'а промежуточной таблицы
	err := s.db.Preload("Roles").First(&user, targetUserID).Error
	if err != nil {
		return nil, errors.New("пользователь не найден")
	}

	var dtos []dto.RoleDTO
	for _, r := range user.Roles {
		var desc string
		if r.Description != nil {
			desc = *r.Description
		}
		dtos = append(dtos, dto.RoleDTO{
			ID:          r.ID,
			Name:        r.Name,
			Slug:        r.Slug,
			Description: desc,
		})
	}

	return dtos, nil
}

// HardRemoveRole - Жесткое удаление роли у пользователя (физически удаляет связь из БД)
func (s *userRoleServiceImpl) HardRemoveRole(targetUserID uint, roleID uint) error {
	// Unscoped() отключает защиту GORM и удаляет строку навсегда
	return s.db.Unscoped().
		Where("user_id = ? AND role_id = ?", targetUserID, roleID).
		Delete(&repository.UserRole{}).Error
}

// SoftRemoveRole - Мягкое удаление (проставляет deleted_at и deleted_by)
func (s *userRoleServiceImpl) SoftRemoveRole(targetUserID uint, roleID uint, currentUserID uint) error {
	// Сначала записываем, КТО удалил эту связь
	err := s.db.Model(&repository.UserRole{}).
		Where("user_id = ? AND role_id = ?", targetUserID, roleID).
		Update("deleted_by", currentUserID).Error
	if err != nil {
		return err
	}

	// Затем вызываем мягкое удаление (GORM сам поставит время в deleted_at)
	return s.db.
		Where("user_id = ? AND role_id = ?", targetUserID, roleID).
		Delete(&repository.UserRole{}).Error
}

// RestoreUserRole - Восстановление мягко удаленной связи
func (s *userRoleServiceImpl) RestoreUserRole(targetUserID uint, roleID uint) error {
	// Unscoped() нужен, чтобы найти даже "удаленную" запись
	// Обнуляем поля deleted_at и deleted_by
	return s.db.Unscoped().Model(&repository.UserRole{}).
		Where("user_id = ? AND role_id = ?", targetUserID, roleID).
		Updates(map[string]interface{}{
			"deleted_at": nil,
			"deleted_by": nil,
		}).Error
}
