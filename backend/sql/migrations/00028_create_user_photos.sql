-- Фотографии профиля: сжатый оригинал + квадратный аватар 128×128.
--
-- Заменяет упрощённую таблицу user_avatars (одна картинка as-is). По ТЗ
-- лабы «загрузка файлов» нужно хранить ДВЕ производные одного снимка:
--   • original_content — сжатая копия оригинала (даунскейл до разумного
--     размера + перекодирование) — отдаётся только владельцу через
--     защищённый маршрут GET /api/photo/download;
--   • avatar_content    — квадрат 128×128, публичная миниатюра для <img>.
--
-- Почему bytea, а не файловая система:
--   1. «Защита от прямого доступа» (критерий ТЗ) выполняется автоматически:
--      у файла в БД нет URL, его физически нельзя запросить напрямую —
--      только через контроллер с проверкой прав.
--   2. Содержимое переживает рестарты вместе с томом postgres и попадает
--      в дампы — отдельный объект-стор/том не нужен.
-- Размер, реальный тип (по сигнатуре) и пиксельный объём (защита от
-- decompression bomb) валидируются на стороне сервиса перед вставкой.
--
-- Мягкое удаление (deleted_at) даёт восстановимость и историю: при
-- загрузке новой фотографии прежняя помечается удалённой, а не затирается.
-- Частичный уникальный индекс гарантирует ровно одну АКТИВНУЮ запись на
-- пользователя.

-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS user_photos (
    id                    BIGSERIAL PRIMARY KEY,
    user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    original_name         TEXT NOT NULL,              -- имя файла, как его прислал клиент
    description           TEXT,                       -- необязательное описание
    format                TEXT NOT NULL,              -- нормализованный формат хранимого оригинала: 'jpeg' | 'png'
    size_bytes            INTEGER NOT NULL,           -- размер сжатого оригинала в байтах
    width                 INTEGER NOT NULL,           -- размеры сжатого оригинала, px
    height                INTEGER NOT NULL,
    original_content      BYTEA NOT NULL,             -- сжатая копия оригинала
    original_content_type TEXT  NOT NULL,             -- image/jpeg | image/png
    avatar_content        BYTEA NOT NULL,             -- квадратная миниатюра 128×128
    avatar_content_type   TEXT  NOT NULL,
    created_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at            TIMESTAMP WITH TIME ZONE
);

-- Ровно одна активная фотография на пользователя; мягко удалённые
-- (deleted_at IS NOT NULL) под ограничение не попадают.
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_photos_active_user
    ON user_photos (user_id) WHERE deleted_at IS NULL;
-- Выборка истории/мягко удалённых и фильтр архива (WHERE deleted_at IS NULL).
CREATE INDEX IF NOT EXISTS idx_user_photos_deleted_at ON user_photos (deleted_at);

-- Старую таблицу одиночных аватаров убираем: её функциональность целиком
-- поглощена user_photos (avatar_content).
DROP TABLE IF EXISTS user_avatars;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS user_avatars (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    content BYTEA NOT NULL,
    content_type TEXT NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
DROP TABLE IF EXISTS user_photos;
-- +goose StatementEnd
