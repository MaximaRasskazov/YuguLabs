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
	// 1. Меняем RoleID на RoleIDs (тип []uint - срез/массив чисел)
	// 2. В binding добавляем min=1 (чтобы массив не был пустым)
	// 3. dive,gt=0 заставит валидатор "нырнуть" в массив и проверить,
	// что каждый ID внутри больше нуля.
	RoleIDs []uint `json:"role_ids" binding:"required,min=1,dive,gt=0"`
}

// Структура того, что мы отдаем юзеру
type UserPermissionResponse struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

// То, что увидит пользователь при запросе своих ролей
type UserRoleResponse struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

// Структура для чистого вывода разрешений (например, для админа)
type PermissionAdminResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	CreatedByID uint   `json:"created_by_id"`
	DeletedByID *uint  `json:"deleted_by_id"` // Оставляем указатель, так как он может быть null
}