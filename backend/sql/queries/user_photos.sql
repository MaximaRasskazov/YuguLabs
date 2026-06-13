-- Запросы к user_photos. Метаданные (без bytea) отдаются лёгкими
-- SELECT'ами; тяжёлые original_content / avatar_content тянем только
-- там, где реально нужно отдать байты клиенту.

-- name: InsertUserPhoto :one
-- Создаёт новую активную фотографию. RETURNING без bytea — не гоняем
-- блобы обратно после вставки.
INSERT INTO user_photos (
    user_id, original_name, description, format, size_bytes, width, height,
    original_content, original_content_type, avatar_content, avatar_content_type
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, user_id, original_name, description, format, size_bytes, width, height,
          original_content_type, avatar_content_type, created_at, updated_at, deleted_at;

-- name: SoftDeleteActiveUserPhoto :exec
-- Помечает текущую активную фотографию пользователя удалённой. Вызывается
-- и при явном удалении, и перед вставкой новой (чтобы активная осталась одна).
UPDATE user_photos
SET deleted_at = NOW(), updated_at = NOW()
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: GetActiveUserPhotoMeta :one
-- Метаданные активной фотографии (для GET /api/photo). Без bytea.
SELECT id, user_id, original_name, description, format, size_bytes, width, height,
       original_content_type, avatar_content_type, created_at, updated_at
FROM user_photos
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: GetActiveUserAvatar :one
-- Байты квадратной миниатюры 128×128 для публичной отдачи в <img>.
SELECT avatar_content, avatar_content_type, updated_at
FROM user_photos
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: GetActiveUserPhotoVersion :one
-- Лёгкая проверка наличия + версия (updated_at) для cache-busting в /me.
-- Не выгружает байты.
SELECT updated_at
FROM user_photos
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: GetActiveUserOriginal :one
-- Байты сжатого оригинала для защищённого скачивания владельцем.
SELECT original_content, original_content_type, original_name, format
FROM user_photos
WHERE user_id = $1 AND deleted_at IS NULL;

-- name: ListAllActiveUserPhotosMeta :many
-- Метаданные всех активных фотографий (без bytea) с данными владельца —
-- для админского просмотра «все фото». Мягко удалённые исключены.
SELECT
    up.id, up.user_id, up.original_name, up.description, up.format,
    up.size_bytes, up.width, up.height, up.created_at,
    u.email, u.first_name, u.last_name, u.middle_name, u.group_name
FROM user_photos up
JOIN users u ON u.id = up.user_id
WHERE up.deleted_at IS NULL
ORDER BY u.last_name, u.first_name, up.id;

-- name: ListActiveUserPhotosForArchive :many
-- Все активные фотографии с данными владельца — для админского архива.
-- Мягко удалённые исключены (deleted_at IS NULL).
SELECT
    up.id, up.user_id, up.original_name, up.format, up.size_bytes,
    up.original_content, up.original_content_type,
    up.avatar_content, up.avatar_content_type, up.created_at,
    u.email, u.first_name, u.last_name, u.middle_name, u.group_name
FROM user_photos up
JOIN users u ON u.id = up.user_id
WHERE up.deleted_at IS NULL
ORDER BY u.last_name, u.first_name, up.id;
