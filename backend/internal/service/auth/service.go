// Package auth реализует регистрацию, вход, refresh и logout пользователей.
//
// Сервис тонкий: проверяет уникальность email, хеширует пароль через
// bcrypt, делегирует выдачу токенов TokenService и атомарно через UoW
// связывает нового пользователя с ролью student.
package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/emulator"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/token"
)

// Дефолтная роль, выдаваемая при регистрации. После регистрации
// пользователь может подать заявку на роль teacher (отдельный поток).
const defaultRoleSlug = "student"

// MinPasswordLength — минимальная длина пароля при регистрации и смене.
// На фронте дублируется через zod-схему.
const MinPasswordLength = 8

// emailRegex — упрощённая проверка email-формата. Финальную валидацию
// делает БД (UNIQUE-индекс) и провайдер при отправке писем.
var emailRegex = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Sentinel-ошибки, на которые HTTP-слой маппит коды ответа.
var (
	ErrEmailTaken          = errors.New("auth: email уже зарегистрирован")
	ErrInvalidEmail        = errors.New("auth: некорректный email")
	ErrPasswordTooShort    = errors.New("auth: пароль слишком короткий")
	ErrInvalidCredentials  = errors.New("auth: неверный email или пароль")
	ErrUserNotFound        = errors.New("auth: пользователь не найден")
	ErrInvalidPassword     = errors.New("auth: текущий пароль неверен")
	ErrSamePassword        = errors.New("auth: новый пароль совпадает с текущим")
	ErrEmulatorUnavailable = errors.New("auth: сервис деканата временно недоступен")
)

// EmulatorAuth — то, что auth-слою нужно от эмулятора: проверка
// учётных данных. Интерфейс (а не *emulator.Client) развязывает
// зависимость и упрощает тесты. nil = эмулятор-логин отключён.
type EmulatorAuth interface {
	AuthCheck(ctx context.Context, email, password string) (string, error)
}

// Service — обёртка над хранилищем и TokenService.
type Service struct {
	store     *repo.Store
	tokens    *token.Service
	emulator  EmulatorAuth       // может быть nil — тогда эмулятор-логин отключён
	changelog *changelog.Service // может быть nil — тогда мутации не пишутся в change_logs
}

// New собирает Service. emu может быть nil (тесты, отсутствие
// EMULATOR_URL) — тогда вход возможен только для локальных юзеров.
func New(store *repo.Store, tokens *token.Service, emu EmulatorAuth) *Service {
	return &Service{store: store, tokens: tokens, emulator: emu}
}

// SetChangelog подключает запись истории мутаций пользователя в change_logs.
// Вынесено в сеттер (а не параметр New), чтобы не ломать существующие
// вызовы и тесты. nil = логирование выключено.
func (s *Service) SetChangelog(c *changelog.Service) { s.changelog = c }

// RegisterInput — параметры регистрации. MiddleName/Birthday/GroupName
// опциональны (студенту нужна группа, но мы не валидируем это здесь —
// форма на фронте сама подскажет).
type RegisterInput struct {
	Email      string
	Password   string
	FirstName  string
	LastName   string
	MiddleName *string
	Birthday   *time.Time
	GroupName  *string
}

// Result — результат успешного Register / Login.
type Result struct {
	User queries.User
	Pair *token.Pair
}

// Profile — данные для GET /api/auth/me.
// Раздаёт минимум, нужный фронту для отрисовки UI и RBAC-проверок.
type Profile struct {
	User            queries.User
	Roles           []queries.Role
	Permissions     []string   // slug'и активных permission'ов
	AvatarUpdatedAt *time.Time // время загрузки аватара (версия для URL), nil если нет
}

// Register создаёт нового пользователя со студентской ролью и сразу
// выдаёт пару токенов. ip пробрасывается в access_tokens для аудита.
func (s *Service) Register(ctx context.Context, in RegisterInput, ip string) (*Result, error) {
	if err := validateRegister(in); err != nil {
		return nil, err
	}
	emailLower := strings.ToLower(strings.TrimSpace(in.Email))

	// Дубликат по email отсекаем заранее — иначе bcrypt напрасно сожгёт CPU.
	if _, err := s.store.GetUserByEmail(ctx, emailLower); err == nil {
		return nil, ErrEmailTaken
	} else if !repo.IsNotFound(err) {
		return nil, fmt.Errorf("lookup email: %w", err)
	}

	passHash, err := hashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	studentRole, err := s.store.GetRoleBySlug(ctx, defaultRoleSlug)
	if err != nil {
		return nil, fmt.Errorf("lookup default role %q: %w", defaultRoleSlug, err)
	}

	var created queries.User
	err = s.store.RunInTx(ctx, func(q *queries.Queries) error {
		u, err := q.CreateUser(ctx, queries.CreateUserParams{
			Email:        emailLower,
			PasswordHash: passHash,
			FirstName:    strings.TrimSpace(in.FirstName),
			LastName:     strings.TrimSpace(in.LastName),
			MiddleName:   in.MiddleName,
			Birthday:     dateOrNull(in.Birthday),
			GroupName:    in.GroupName,
		})
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		_, err = q.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
			UserID:    u.ID,
			RoleID:    studentRole.ID,
			CreatedBy: u.ID, // сам себя «создал» — для системного актора удобнее, чем NULL
		})
		if err != nil {
			return fmt.Errorf("attach default role: %w", err)
		}
		if s.changelog != nil {
			uid := pgutil.UUID(u.ID)
			if err := s.changelog.LogCreatedTx(ctx, q, changelog.EntityUser, uid.String(), changelog.UserSnapshot(u), uid); err != nil {
				return fmt.Errorf("changelog created: %w", err)
			}
		}
		created = u
		return nil
	})
	if err != nil {
		return nil, err
	}

	pair, err := s.tokens.Issue(ctx, pgutil.UUID(created.ID), ip)
	if err != nil {
		return nil, fmt.Errorf("issue tokens: %w", err)
	}
	return &Result{User: created, Pair: pair}, nil
}

// Login проверяет email+password и выдаёт пару токенов.
//
// Две ветки проверки пароля:
//   - локальный юзер (есть password_hash): bcrypt-сравнение, как обычно.
//     Это admin, dean и самозарегистрированные.
//   - эмуляторный юзер (password_hash пустой): пароль не хранится у нас,
//     спрашиваем эмулятор через AuthCheck. Так логинятся студенты и
//     преподаватели, импортированные из деканата.
func (s *Service) Login(ctx context.Context, email, password, ip string) (*Result, error) {
	emailLower := strings.ToLower(strings.TrimSpace(email))

	user, err := s.store.GetUserByEmail(ctx, emailLower)
	if err != nil {
		if repo.IsNotFound(err) {
			_ = checkPassword(dummyHash, password) // выравниваем время
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}

	if user.PasswordHash == "" {
		// Эмуляторный юзер — проверяем пароль на стороне деканата.
		if err := s.loginViaEmulator(ctx, user, password); err != nil {
			return nil, err
		}
	} else {
		// Локальный юзер — обычная bcrypt-проверка.
		if err := checkPassword(user.PasswordHash, password); err != nil {
			return nil, ErrInvalidCredentials
		}
	}

	pair, err := s.tokens.Issue(ctx, pgutil.UUID(user.ID), ip)
	if err != nil {
		return nil, fmt.Errorf("issue tokens: %w", err)
	}
	return &Result{User: user, Pair: pair}, nil
}

// loginViaEmulator проверяет пароль эмуляторного юзера через деканат.
// Сетевую/любую неожиданную ошибку трактуем как «эмулятор недоступен»
// (мягкий вариант) — чтобы на защите при моргнувшем эмуляторе юзер
// увидел понятное «сервис недоступен», а не «неверный пароль».
func (s *Service) loginViaEmulator(ctx context.Context, user queries.User, password string) error {
	if s.emulator == nil {
		// Эмулятор-логин отключён, а локального пароля нет — войти нельзя.
		return ErrInvalidCredentials
	}
	// queries.User не селектит external_id, поэтому возвращённый эмулятором
	// идентификатор не сверяем: email уникален, а эмулятор подтвердил пароль
	// именно для этого email — связки по email достаточно.
	extID, err := s.emulator.AuthCheck(ctx, user.Email, password)
	if err != nil {
		if errors.Is(err, emulator.ErrAuthInvalid) {
			return ErrInvalidCredentials
		}
		return ErrEmulatorUnavailable
	}
	_ = extID
	return nil
}

// Refresh — прокси к TokenService.Rotate. Здесь нужен сервис auth
// в основном для единообразия (handler работает с одним сервисом).
func (s *Service) Refresh(ctx context.Context, refreshPlain, ip string) (*token.Pair, error) {
	return s.tokens.Rotate(ctx, refreshPlain, ip)
}

// Logout закрывает текущую сессию (точечный revoke access-токена).
func (s *Service) Logout(ctx context.Context, accessTokenID uuid.UUID) error {
	return s.tokens.Revoke(ctx, accessTokenID)
}

// Me возвращает профиль пользователя с ролями и плоским списком
// активных permission-slug'ов. Permission-список фронт может
// использовать для условного рендеринга кнопок.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (*Profile, error) {
	pgID := pgutil.PgUUID(userID)

	user, err := s.store.GetUserByID(ctx, pgID)
	if err != nil {
		if repo.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	roles, err := s.store.ListRolesForUser(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	perms, err := s.store.ListPermissionsForUser(ctx, pgID)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}

	return &Profile{
		User:            user,
		Roles:           roles,
		Permissions:     perms,
		AvatarUpdatedAt: s.avatarUpdatedAt(ctx, pgID),
	}, nil
}

func validateRegister(in RegisterInput) error {
	if !emailRegex.MatchString(strings.TrimSpace(in.Email)) {
		return ErrInvalidEmail
	}
	if len(in.Password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if strings.TrimSpace(in.FirstName) == "" || strings.TrimSpace(in.LastName) == "" {
		return fmt.Errorf("auth: first_name и last_name обязательны")
	}
	return nil
}

func dateOrNull(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
