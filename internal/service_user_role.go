package service

import (
	"errors"
	"time"
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
	AssignRoles(targetUserID uint, roleIDs []uint, currentUserID uint) error
	GetUserPermissions(userID uint) ([]repository.Permission, error)
}

type userRoleServiceImpl struct {
	db *gorm.DB
}

func NewUserRoleService(db *gorm.DB) UserRoleService {
	return &userRoleServiceImpl{db: db}
}

// AssignRole привязывает роль к пользователю (С ЗАЩИТОЙ ОТ КЛОНОВ)
func (s *userRoleServiceImpl) AssignRole(targetUserID uint, roleID uint, currentUserID uint) error {
	var userCount, roleCount int64
	s.db.Model(&repository.User{}).Where("id = ?", targetUserID).Count(&userCount)
	s.db.Model(&repository.Role{}).Where("id = ?", roleID).Count(&roleCount)

	if userCount == 0 {
		return errors.New("пользователь не найден")
	}
	if roleCount == 0 {
		return errors.New("роль не найдена")
	}

	var existingLink repository.UserRole
	err := s.db.Unscoped().Where("user_id = ? AND role_id = ?", targetUserID, roleID).First(&existingLink).Error

	if err == nil {
		if !existingLink.DeletedAt.Valid {
			return errors.New("пользователь уже имеет эту активную роль")
		}
		return s.db.Unscoped().Model(&existingLink).Updates(map[string]interface{}{
			"deleted_at": nil,
			"deleted_by": nil,
		}).Error
	}

	userRole := repository.UserRole{
		UserID:      targetUserID,
		RoleID:      roleID,
		CreatedByID: currentUserID,
	}

	return s.db.Create(&userRole).Error
}

// GetUserRoles возвращает список активных ролей конкретного пользователя
func (s *userRoleServiceImpl) GetUserRoles(targetUserID uint) ([]dto.RoleDTO, error) {
	var userCount int64
	s.db.Model(&repository.User{}).Where("id = ?", targetUserID).Count(&userCount)
	if userCount == 0 {
		return nil, errors.New("пользователь не найден")
	}

	var roles []repository.Role

	err := s.db.Table("roles").
		Select("DISTINCT roles.*").
		Joins("JOIN role_user ON role_user.role_id = roles.id").
		Where("role_user.user_id = ? AND role_user.deleted_at IS NULL AND roles.deleted_at IS NULL", targetUserID).
		Find(&roles).Error
	if err != nil {
		return nil, errors.New("ошибка при получении ролей")
	}

	var dtos []dto.RoleDTO
	for _, r := range roles {
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

// HardRemoveRole - Жесткое удаление роли у пользователя
func (s *userRoleServiceImpl) HardRemoveRole(targetUserID uint, roleID uint) error {
	result := s.db.Unscoped().
		Where("user_id = ? AND role_id = ?", targetUserID, roleID).
		Delete(&repository.UserRole{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("связь не найдена (нечего удалять)")
	}

	return nil
}

func (s *userRoleServiceImpl) SoftRemoveRole(targetUserID uint, roleID uint, currentUserID uint) error {
	result := s.db.Model(&repository.UserRole{}).
		Where("user_id = ? AND role_id = ? AND deleted_at IS NULL", targetUserID, roleID).
		Updates(map[string]interface{}{
			"deleted_at": time.Now(),
			"deleted_by": currentUserID,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("активная связь не найдена (возможно, уже удалена)")
	}

	return nil
}

// RestoreUserRole - Восстановление мягко удаленной связи
func (s *userRoleServiceImpl) RestoreUserRole(targetUserID uint, roleID uint) error {
	result := s.db.Unscoped().Model(&repository.UserRole{}).
		Where("user_id = ? AND role_id = ?", targetUserID, roleID).
		Updates(map[string]interface{}{
			"deleted_at": nil,
			"deleted_by": nil,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("связь не найдена или была удалена навсегда")
	}

	return nil
}

// AssignRoles - Массовая выдача ролей
func (s *userRoleServiceImpl) AssignRoles(targetUserID uint, roleIDs []uint, currentUserID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, roleID := range roleIDs {
			link := repository.UserRole{
				UserID:      targetUserID,
				RoleID:      roleID,
				CreatedByID: currentUserID,
			}
			if err := tx.Where(repository.UserRole{UserID: targetUserID, RoleID: roleID}).
				FirstOrCreate(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetUserPermissions - Получение всех уникальных разрешений юзера (Идеальный запрос)
func (s *userRoleServiceImpl) GetUserPermissions(targetUserID uint) ([]repository.Permission, error) {
	var permissions []repository.Permission

	err := s.db.Table("permissions").
		Select("DISTINCT permissions.*").
		Joins("JOIN permission_role ON permission_role.permission_id = permissions.id").
		Joins("JOIN role_user ON role_user.role_id = permission_role.role_id").
		Where("role_user.user_id = ? AND role_user.deleted_at IS NULL AND permission_role.deleted_at IS NULL AND permissions.deleted_at IS NULL", targetUserID).
		Find(&permissions).Error

	return permissions, err
}
