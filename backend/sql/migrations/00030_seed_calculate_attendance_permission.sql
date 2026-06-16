-- Право на авто-расчёт зачёта по файлу успеваемости (лаба №12) —
-- POST /api/attendance/calculate. Аналог разрешения calculate-attendance
-- из ТЗ (раздел «Разрешения»). Группа UUID a000000a-* — следующая
-- свободная после a0000009 photos.*. Привязываем явно к роли админа:
-- общий bulk-INSERT «все системные права админу» отработал лишь в 00010
-- на свой момент времени и новые права не подхватывает.

-- +goose Up
-- +goose StatementBegin
INSERT INTO permissions (id, name, slug, description, is_system, created_at, created_by)
VALUES (
    'a000000a-0000-0000-0000-000000000001',
    'Attendance: Calculate',
    'calculate-attendance',
    'Авто-расчёт зачёта по файлу успеваемости (.xlsx)',
    TRUE, NOW(), '00000000-0000-0000-0000-000000000000'
)
ON CONFLICT (id) DO NOTHING;

-- Выдаём администратору (роль 11111111-...). Идемпотентно.
INSERT INTO permission_role (role_id, permission_id, created_at, created_by)
SELECT '11111111-1111-1111-1111-111111111111',
       'a000000a-0000-0000-0000-000000000001',
       NOW(), '00000000-0000-0000-0000-000000000000'
WHERE NOT EXISTS (
    SELECT 1 FROM permission_role pr
    WHERE pr.role_id = '11111111-1111-1111-1111-111111111111'
      AND pr.permission_id = 'a000000a-0000-0000-0000-000000000001'
      AND pr.deleted_at IS NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM permission_role WHERE permission_id = 'a000000a-0000-0000-0000-000000000001';
DELETE FROM permissions WHERE id = 'a000000a-0000-0000-0000-000000000001';
-- +goose StatementEnd
