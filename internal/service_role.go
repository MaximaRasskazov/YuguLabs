package service

import (
	"errors"
	"time"
	"yugu-server/internal/dto"
	"yugu-server/internal/repository"

	"gorm.io/gorm"
)

type RoleService interface {
	CreateRole(req dto.StoreRoleRequest, userID uint) (dto.RoleDTO, error)
	GetRoles() ([]dto.RoleDTO, error)

	SoftDeleteRole(roleID uint, currentUserID uint) error
	RestoreRole(roleID uint) error
	HardDeleteRole(roleID uint) error

	UpdateRole(roleID uint, req dto.UpdateRoleRequest) (dto.RoleDTO, error)
	AssignPermissionToRole(roleID uint, permID uint) error
	GetPermissionsByRoleID(roleID uint) ([]repository.Permission, error)

	// ДОБАВЛЕНО: Интерфейс теперь знает об этой функции
	RevokePermission(roleID uint, permissionID uint) error
}

type roleServiceImpl struct {
	db *gorm.DB
}

func NewRoleService(db *gorm.DB) RoleService {
	return &roleServiceImpl{db: db}
}

// CreateRole создает новую роль и автоматически проставляет created_by
func (s *roleServiceImpl) CreateRole(req dto.StoreRoleRequest, userID uint) (dto.RoleDTO, error) {
	var count int64
	s.db.Model(&repository.Role{}).Where("name = ? OR slug = ?", req.Name, req.Slug).Count(&count)
	if count > 0 {
		return dto.RoleDTO{}, errors.New("роль с таким именем или slug уже существует")
	}

	role := repository.Role{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		CreatedByID: userID,
	}

	if err := s.db.Create(&role).Error; err != nil {
		return dto.RoleDTO{}, err
	}

	var desc string
	if role.Description != nil {
		desc = *role.Description
	}

	return dto.RoleDTO{
		ID:          role.ID,
		Name:        role.Name,
		Slug:        role.Slug,
		Description: desc,
	}, nil
}

// GetRoles возвращает список всех активных ролей
func (s *roleServiceImpl) GetRoles() ([]dto.RoleDTO, error) {
	var roles []repository.Role
	if err := s.db.Find(&roles).Error; err != nil {
		return nil, err
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

// SoftDeleteRole помечает роль как удаленную
func (s *roleServiceImpl) SoftDeleteRole(roleID uint, currentUserID uint) error {
	if err := s.db.Model(&repository.Role{}).Where("id = ?", roleID).Update("deleted_by", currentUserID).Error; err != nil {
		return err
	}

	return s.db.Delete(&repository.Role{}, roleID).Error
}

func (s *roleServiceImpl) RestoreRole(roleID uint) error {
	result := s.db.Unscoped().Model(&repository.Role{}).Where("id = ?", roleID).Updates(map[string]interface{}{
		"deleted_at": nil,
		"deleted_by": nil,
	})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("роль не найдена или была удалена навсегда")
	}

	return nil
}

// HardDeleteRole физически удаляет запись из БД
func (s *roleServiceImpl) HardDeleteRole(roleID uint) error {
	return s.db.Unscoped().Delete(&repository.Role{}, roleID).Error
}

func (s *roleServiceImpl) UpdateRole(roleID uint, req dto.UpdateRoleRequest) (dto.RoleDTO, error) {
	var role repository.Role

	if err := s.db.First(&role, roleID).Error; err != nil {
		return dto.RoleDTO{}, errors.New("роль не найдена")
	}

	var count int64
	s.db.Model(&repository.Role{}).
		Where("(name = ? OR slug = ?) AND id != ?", req.Name, req.Slug, roleID).
		Count(&count)

	if count > 0 {
		return dto.RoleDTO{}, errors.New("другая роль с таким именем или slug уже существует")
	}

	if req.Name != "" {
		role.Name = req.Name
	}
	if req.Slug != "" {
		role.Slug = req.Slug
	}
	if req.Description != nil {
		role.Description = req.Description
	}

	if err := s.db.Save(&role).Error; err != nil {
		return dto.RoleDTO{}, err
	}

	var desc string
	if role.Description != nil {
		desc = *role.Description
	}

	return dto.RoleDTO{
		ID:          role.ID,
		Name:        role.Name,
		Slug:        role.Slug,
		Description: desc,
	}, nil
}

func (s *roleServiceImpl) AssignPermissionToRole(roleID uint, permID uint) error {
	// 1. Проверяем, есть ли уже активная связь
	var activeCount int64
	s.db.Table("permission_role").
		Where("role_id = ? AND permission_id = ? AND deleted_at IS NULL", roleID, permID).
		Count(&activeCount)

	if activeCount > 0 {
		return errors.New("это разрешение уже привязано к роли")
	}

	// 2. Проверяем, была ли связь мягко удалена ранее (лежит в корзине)
	var deletedCount int64
	s.db.Table("permission_role").
		Where("role_id = ? AND permission_id = ? AND deleted_at IS NOT NULL", roleID, permID).
		Count(&deletedCount)

	if deletedCount > 0 {
		// Восстанавливаем связь (очищаем deleted_at)
		return s.db.Table("permission_role").
			Where("role_id = ? AND permission_id = ?", roleID, permID).
			Update("deleted_at", nil).Error
	}

	// 3. Если связи вообще никогда не было — создаем новую
	return s.db.Table("permission_role").Create(map[string]interface{}{
		"role_id":       roleID,
		"permission_id": permID,
		"created_by":    1,
		"created_at":    time.Now(),
	}).Error
}

func (s *roleServiceImpl) GetPermissionsByRoleID(roleID uint) ([]repository.Permission, error) {
	var permissions []repository.Permission

	err := s.db.Table("permissions").
		Joins("INNER JOIN permission_role ON permissions.id = permission_role.permission_id").
		Where("permission_role.role_id = ? AND permission_role.deleted_at IS NULL", roleID).
		Find(&permissions).Error

	return permissions, err
}

// ДОБАВЛЕНО И ИСПРАВЛЕНО: Мягкое удаление связи через Table
func (s *roleServiceImpl) RevokePermission(roleID, permissionID uint) error {
	return s.db.Table("permission_role").
		Where("role_id = ? AND permission_id = ?", roleID, permissionID).
		Update("deleted_at", time.Now()).Error
}
