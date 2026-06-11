-- name: CreateChangeLog :one
INSERT INTO change_logs (entity_type, entity_id, action, before, after, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetChangeLogByID :one
SELECT *
FROM change_logs
WHERE id = $1;

-- name: ListChangeLogsForEntity :many
-- История изменений конкретной сущности — лента "что менялось у этого
-- долга / пересдачи / роли". Используется при выводе story-страницы.
SELECT *
FROM change_logs
WHERE entity_type = $1
  AND entity_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListChangeLogsByEntityType :many
SELECT *
FROM change_logs
WHERE entity_type = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListChangeLogsByAuthor :many
SELECT *
FROM change_logs
WHERE created_by = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListRecentChangeLogs :many
-- Общий журнал последних изменений для админ-вкладки «Журнал изменений».
-- Только сущности users/roles/permissions (доменные debt/retake/discipline
-- сюда не тянем — это техника, не для человека). Зеркальные role-side
-- записи смены роли скрыты (операция видна со стороны пользователя).
-- К user-записям подтягиваем ФИО, чтобы лента читалась без UUID.
SELECT
    cl.id, cl.entity_type, cl.entity_id, cl.action, cl.before, cl.after, cl.created_at, cl.created_by,
    u.last_name   AS subject_last_name,
    u.first_name  AS subject_first_name,
    u.middle_name AS subject_middle_name
FROM change_logs cl
LEFT JOIN users u ON cl.entity_type = 'user' AND u.id = cl.entity_id::uuid
WHERE cl.entity_type IN ('user', 'role', 'permission')
  AND NOT (cl.entity_type = 'role' AND cl.action IN ('role_assigned', 'role_revoked'))
ORDER BY cl.created_at DESC
LIMIT $1 OFFSET $2;
