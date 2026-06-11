-- name: UpsertUserAvatar :exec
INSERT INTO user_avatars (user_id, content, content_type, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (user_id) DO UPDATE
SET content = EXCLUDED.content,
    content_type = EXCLUDED.content_type,
    updated_at = NOW();

-- name: GetUserAvatar :one
SELECT content, content_type, updated_at
FROM user_avatars
WHERE user_id = $1;

-- name: GetUserAvatarMeta :one
-- Лёгкая проверка наличия + версия (updated_at) без выгрузки байтов.
SELECT content_type, updated_at
FROM user_avatars
WHERE user_id = $1;

-- name: DeleteUserAvatar :exec
DELETE FROM user_avatars WHERE user_id = $1;
