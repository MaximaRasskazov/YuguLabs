-- Право на откат (undo) записей из истории изменений. Просмотр истории
-- покрывается уже существующим changelog.view (см. 00010); здесь добавляем
-- отдельное право на восстановление состояния и выдаём его администратору.
-- UUID — в группе a0000007 (audit/changelog), следующий по порядку.

-- +goose Up
-- +goose StatementBegin
INSERT INTO permissions (id, name, slug, description, is_system, created_at, created_by)
VALUES (
    'a0000007-0000-0000-0000-000000000003',
    'Changelog: Restore',
    'changelog.restore',
    'Откат сущности к состоянию из истории изменений (undo)',
    TRUE, NOW(), '00000000-0000-0000-0000-000000000000'
)
ON CONFLICT (id) DO NOTHING;

-- Выдаём администратору (его роль 11111111-...). Идемпотентно.
INSERT INTO permission_role (role_id, permission_id, created_at, created_by)
SELECT '11111111-1111-1111-1111-111111111111',
       'a0000007-0000-0000-0000-000000000003',
       NOW(), '00000000-0000-0000-0000-000000000000'
WHERE NOT EXISTS (
    SELECT 1 FROM permission_role pr
    WHERE pr.role_id = '11111111-1111-1111-1111-111111111111'
      AND pr.permission_id = 'a0000007-0000-0000-0000-000000000003'
      AND pr.deleted_at IS NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM permission_role WHERE permission_id = 'a0000007-0000-0000-0000-000000000003';
DELETE FROM permissions WHERE id = 'a0000007-0000-0000-0000-000000000003';
-- +goose StatementEnd
