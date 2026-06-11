-- Минимальные демо-аккаунты для разработки и тестов фронта/QA.
--
-- Накатываются ОТДЕЛЬНОЙ цепочкой миграций (goose-таблица
-- goose_seed_version, см. entrypoint.sh) — это позволяет легко
-- отключать сиды в prod через SEED_DEV_ACCOUNTS=false, не вмешиваясь
-- в основную последовательность миграций sql/migrations.
--
-- Сидятся admin и dean (этих ролей в эмуляторе деканата нет — нужны, чтобы
-- залогиниться и попасть в админ-панель) плюс набор демо-студентов, чтобы
-- админ-панель была наполнена и без эмулятора (на свежей БД/другом ноуте,
-- где sync ещё не отрабатывал). Реальные студенты/преподаватели по-прежнему
-- приходят из эмулятора через sync.Service; у демо-сидов external_id = NULL,
-- поэтому sync их не трогает, а почты @academic.local не конфликтуют.
--
-- Если убрать и эти два сида — в свежей БД ВООБЩЕ не будет ни одного
-- администратора, и попасть в /api/users, /api/sync/trigger и т.д.
-- будет некому. Создание admin без сидов потребует ручной SQL-вставки
-- после деплоя — это худший UX.
--
-- Состав:
--   admin@academic.local              — роль admin (level 1000)
--   dean@academic.local               — роль dean  (level 700)
--   student1..student8@academic.local — роль student (группы БСБО-01/02/03-22)
--
-- Пароль у всех одинаковый: "password" (bcrypt cost=10).
-- ID детерминированные, чтобы повторный накат на инициализированной БД
-- через ON CONFLICT (id) DO NOTHING не дублировал данные.

-- +goose Up
-- +goose StatementBegin

INSERT INTO users (id, email, password_hash, first_name, last_name, middle_name, group_name)
VALUES
    ('aaaa0000-0000-0000-0000-000000000001', 'admin@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Главный', 'Администратор', NULL,        NULL),
    ('bbbb0000-0000-0000-0000-000000000001', 'dean@academic.local',  '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Иван',    'Деканов',       'Сергеевич', NULL),
    -- Демо-студенты: чтобы админ-панель была наполнена без эмулятора.
    ('cccc0000-0000-0000-0000-000000000001', 'student1@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Анна',      'Смирнова',   'Игоревна',      'БСБО-01-22'),
    ('cccc0000-0000-0000-0000-000000000002', 'student2@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Пётр',      'Иванов',     'Сергеевич',     'БСБО-01-22'),
    ('cccc0000-0000-0000-0000-000000000003', 'student3@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Мария',     'Кузнецова',  'Андреевна',     'БСБО-01-22'),
    ('cccc0000-0000-0000-0000-000000000004', 'student4@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Алексей',   'Попов',      'Дмитриевич',    'БСБО-02-22'),
    ('cccc0000-0000-0000-0000-000000000005', 'student5@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Екатерина', 'Соколова',   'Павловна',      'БСБО-02-22'),
    ('cccc0000-0000-0000-0000-000000000006', 'student6@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Дмитрий',   'Лебедев',    'Олегович',      'БСБО-02-22'),
    ('cccc0000-0000-0000-0000-000000000007', 'student7@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Ольга',     'Новикова',   'Викторовна',    'БСБО-03-22'),
    ('cccc0000-0000-0000-0000-000000000008', 'student8@academic.local', '$2a$10$gMK4lndQ1uTZ0CpVXyqNC.ZB5SAU8QEchkAD81B73OENQv7FQOF1.', 'Сергей',    'Морозов',    'Александрович', 'БСБО-03-22')
ON CONFLICT (id) DO NOTHING;

-- Привязка ролей. created_by — служебный системный user из 00010_seed_rbac.
INSERT INTO role_user (user_id, role_id, created_by)
SELECT m.user_id, m.role_id, '00000000-0000-0000-0000-000000000000'::uuid
FROM (VALUES
    ('aaaa0000-0000-0000-0000-000000000001'::uuid, '11111111-1111-1111-1111-111111111111'::uuid),
    ('bbbb0000-0000-0000-0000-000000000001'::uuid, '22222222-2222-2222-2222-222222222222'::uuid),
    ('cccc0000-0000-0000-0000-000000000001'::uuid, '44444444-4444-4444-4444-444444444444'::uuid),
    ('cccc0000-0000-0000-0000-000000000002'::uuid, '44444444-4444-4444-4444-444444444444'::uuid),
    ('cccc0000-0000-0000-0000-000000000003'::uuid, '44444444-4444-4444-4444-444444444444'::uuid),
    ('cccc0000-0000-0000-0000-000000000004'::uuid, '44444444-4444-4444-4444-444444444444'::uuid),
    ('cccc0000-0000-0000-0000-000000000005'::uuid, '44444444-4444-4444-4444-444444444444'::uuid),
    ('cccc0000-0000-0000-0000-000000000006'::uuid, '44444444-4444-4444-4444-444444444444'::uuid),
    ('cccc0000-0000-0000-0000-000000000007'::uuid, '44444444-4444-4444-4444-444444444444'::uuid),
    ('cccc0000-0000-0000-0000-000000000008'::uuid, '44444444-4444-4444-4444-444444444444'::uuid)
) AS m(user_id, role_id)
WHERE NOT EXISTS (
    SELECT 1 FROM role_user ru
    WHERE ru.user_id = m.user_id
      AND ru.role_id = m.role_id
      AND ru.deleted_at IS NULL
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_user WHERE user_id IN (
    'aaaa0000-0000-0000-0000-000000000001',
    'bbbb0000-0000-0000-0000-000000000001',
    'cccc0000-0000-0000-0000-000000000001',
    'cccc0000-0000-0000-0000-000000000002',
    'cccc0000-0000-0000-0000-000000000003',
    'cccc0000-0000-0000-0000-000000000004',
    'cccc0000-0000-0000-0000-000000000005',
    'cccc0000-0000-0000-0000-000000000006',
    'cccc0000-0000-0000-0000-000000000007',
    'cccc0000-0000-0000-0000-000000000008'
);
DELETE FROM users WHERE id IN (
    'aaaa0000-0000-0000-0000-000000000001',
    'bbbb0000-0000-0000-0000-000000000001',
    'cccc0000-0000-0000-0000-000000000001',
    'cccc0000-0000-0000-0000-000000000002',
    'cccc0000-0000-0000-0000-000000000003',
    'cccc0000-0000-0000-0000-000000000004',
    'cccc0000-0000-0000-0000-000000000005',
    'cccc0000-0000-0000-0000-000000000006',
    'cccc0000-0000-0000-0000-000000000007',
    'cccc0000-0000-0000-0000-000000000008'
);
-- +goose StatementEnd
