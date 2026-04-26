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
}

type roleServiceImpl struct {
	db *gorm.DB
}

func NewRoleService(db *gorm.DB) RoleService {
	return &roleServiceImpl{db: db}
}

// CreateRole создает новую роль и автоматически проставляет created_by
func (s *roleServiceImpl) CreateRole(req dto.StoreRoleRequest, userID uint) (dto.RoleDTO, error) {
	// 1. Проверяем уникальность Name и Slug (чтобы не было дублей)
	var count int64
	s.db.Model(&repository.Role{}).Where("name = ? OR slug = ?", req.Name, req.Slug).Count(&count)
	if count > 0 {
		return dto.RoleDTO{}, errors.New("роль с таким именем или slug уже существует")
	}

	// 2. Создаем модель БД. Вот здесь мы выполняем требование ТЗ про created_by!
	role := repository.Role{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		CreatedByID: userID, // Берем ID того, кто отправил запрос
	}

	if err := s.db.Create(&role).Error; err != nil {
		return dto.RoleDTO{}, err
	}

	// 3. Возвращаем безопасный DTO (без служебных полей)
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
	// GORM автоматически добавит "WHERE deleted_at IS NULL" благодаря SoftDeletes
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

// SoftDeleteRole помечает роль как удаленную и записывает, кто это сделал
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

	// Ищем существующую роль
	if err := s.db.First(&role, roleID).Error; err != nil {
		return dto.RoleDTO{}, errors.New("роль не найдена")
	}

	// Каверзный момент ТЗ: проверяем уникальность, ИСКЛЮЧАЯ текущий ID
	var count int64
	s.db.Model(&repository.Role{}).
		Where("(name = ? OR slug = ?) AND id != ?", req.Name, req.Slug, roleID).
		Count(&count)

	if count > 0 {
		return dto.RoleDTO{}, errors.New("другая роль с таким именем или slug уже существует")
	}

	// Обновляем поля, если они пришли в запросе
	if req.Name != "" {
		role.Name = req.Name
	}
	if req.Slug != "" {
		role.Slug = req.Slug
	}
	if req.Description != nil {
		role.Description = req.Description
	}

	// Сохраняем изменения в БД
	if err := s.db.Save(&role).Error; err != nil {
		return dto.RoleDTO{}, err
	}

	// Возвращаем DTO
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
	// Оставили только те поля, которые реально существуют в таблице permission_role
	return s.db.Table("permission_role").Create(map[string]interface{}{
		"role_id":       roleID,
		"permission_id": permID,
		"created_by":    1, // ID админа для лабы
		"created_at":    time.Now(),
	}).Error
}

func (s *roleServiceImpl) GetPermissionsByRoleID(roleID uint) ([]repository.Permission, error) {
	var permissions []repository.Permission

	err := s.db.Table("permissions").
		Joins("INNER JOIN permission_role ON permissions.id = permission_role.permission_id").
		Where("permission_role.role_id = ?", roleID).
		Find(&permissions).Error

	return permissions, err
}