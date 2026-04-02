package service

import (
	"errors"
	"yugu-server/internal/dto"
	"yugu-server/internal/repository"

	"gorm.io/gorm"
)

type PermissionService interface {
	CreatePermission(req dto.StorePermissionRequest, userID uint) (dto.PermissionDTO, error)
	GetPermissions() ([]dto.PermissionDTO, error)
	UpdatePermission(permID uint, req dto.UpdateRoleRequest) (dto.PermissionDTO, error) // Используем тот же DTO для обновления
	HardDeletePermission(permID uint) error
	SoftDeletePermission(permID uint, currentUserID uint) error
	RestorePermission(permID uint) error
}

type permissionServiceImpl struct {
	db *gorm.DB
}

func NewPermissionService(db *gorm.DB) PermissionService {
	return &permissionServiceImpl{db: db}
}

func (s *permissionServiceImpl) CreatePermission(req dto.StorePermissionRequest, userID uint) (dto.PermissionDTO, error) {
	var count int64
	s.db.Model(&repository.Permission{}).Where("name = ? OR slug = ?", req.Name, req.Slug).Count(&count)
	if count > 0 {
		return dto.PermissionDTO{}, errors.New("разрешение с таким именем или slug уже существует")
	}

	perm := repository.Permission{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		CreatedByID: userID,
	}

	if err := s.db.Create(&perm).Error; err != nil {
		return dto.PermissionDTO{}, err
	}

	var desc string
	if perm.Description != nil {
		desc = *perm.Description
	}

	return dto.PermissionDTO{ID: perm.ID, Name: perm.Name, Slug: perm.Slug, Description: desc}, nil
}

func (s *permissionServiceImpl) GetPermissions() ([]dto.PermissionDTO, error) {
	var perms []repository.Permission
	if err := s.db.Find(&perms).Error; err != nil {
		return nil, err
	}

	var dtos []dto.PermissionDTO
	for _, p := range perms {
		var desc string
		if p.Description != nil {
			desc = *p.Description
		}
		dtos = append(dtos, dto.PermissionDTO{ID: p.ID, Name: p.Name, Slug: p.Slug, Description: desc})
	}
	return dtos, nil
}

func (s *permissionServiceImpl) UpdatePermission(permID uint, req dto.UpdateRoleRequest) (dto.PermissionDTO, error) {
	var perm repository.Permission
	if err := s.db.First(&perm, permID).Error; err != nil {
		return dto.PermissionDTO{}, errors.New("разрешение не найдено")
	}

	var count int64
	s.db.Model(&repository.Permission{}).Where("(name = ? OR slug = ?) AND id != ?", req.Name, req.Slug, permID).Count(&count)
	if count > 0 {
		return dto.PermissionDTO{}, errors.New("другое разрешение с таким именем или slug уже существует")
	}

	if req.Name != "" {
		perm.Name = req.Name
	}
	if req.Slug != "" {
		perm.Slug = req.Slug
	}
	if req.Description != nil {
		perm.Description = req.Description
	}

	if err := s.db.Save(&perm).Error; err != nil {
		return dto.PermissionDTO{}, err
	}

	var desc string
	if perm.Description != nil {
		desc = *perm.Description
	}
	return dto.PermissionDTO{ID: perm.ID, Name: perm.Name, Slug: perm.Slug, Description: desc}, nil
}

func (s *permissionServiceImpl) HardDeletePermission(permID uint) error {
	return s.db.Unscoped().Delete(&repository.Permission{}, permID).Error
}

func (s *permissionServiceImpl) SoftDeletePermission(permID uint, currentUserID uint) error {
	if err := s.db.Model(&repository.Permission{}).Where("id = ?", permID).Update("deleted_by", currentUserID).Error; err != nil {
		return err
	}
	return s.db.Delete(&repository.Permission{}, permID).Error
}

func (s *permissionServiceImpl) RestorePermission(permID uint) error {
	return s.db.Unscoped().Model(&repository.Permission{}).Where("id = ?", permID).
		Updates(map[string]interface{}{"deleted_at": nil, "deleted_by": nil}).Error
}
