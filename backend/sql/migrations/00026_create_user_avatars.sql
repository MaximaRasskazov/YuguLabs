-- Аватары пользователей.
--
-- Вынесены в отдельную таблицу (а не колонкой в users), чтобы бинарь не
-- тянулся в каждый SELECT * по users (логин, списки студентов, отчёты).
-- Хранится прямо в БД (bytea): переживает рестарты вместе с томом postgres
-- и попадает в дампы — отдельный объект-стор/том не нужен. Размер и MIME
-- валидируются на стороне сервиса (≤2 МБ, jpeg/png/webp).

-- +goose Up
CREATE TABLE IF NOT EXISTS user_avatars (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    content BYTEA NOT NULL,
    content_type TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS user_avatars;
