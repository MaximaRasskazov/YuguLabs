package dto

// StoreRoleRequest — валидация при создании роли (POST /api/ref/policy/role)
type StoreRoleRequest struct {
	Name        string  `json:"name" binding:"required,min=2"`
	Slug        string  `json:"slug" binding:"required,slug_format"` // Используем кастомное правило slug_format
	Description *string `json:"description"`                         // Указатель, так как поле необязательное
}

// UpdateRoleRequest — валидация при обновлении роли (PUT/PATCH)
type UpdateRoleRequest struct {
	Name        string  `json:"name" binding:"omitempty,min=2"`
	Slug        string  `json:"slug" binding:"omitempty,slug_format"`
	Description *string `json:"description"`
}

// StorePermissionRequest — валидация при создании разрешения (POST /api/ref/policy/permission)
type StorePermissionRequest struct {
	Name        string  `json:"name" binding:"required,min=2"`
	Slug        string  `json:"slug" binding:"required,slug_format"`
	Description *string `json:"description"`
}

// AttachUserRoleRequest — валидация при привязке роли к пользователю
type AttachUserRoleRequest struct {
	RoleID uint `json:"role_id" binding:"required,gt=0"` // Должно быть больше нуля
}
