// Package teacher_request реализует заявки на роль преподавателя и HTTP-API
// управления ролями/правами (RBAC API).
//
// Жизненный цикл заявки: pending → approved | rejected.
// При одобрении в одной транзакции: статус approved + AttachRoleToUser(teacher).
// При отклонении decision_reason обязателен (enforced БД + сервисом).
package teacher_request

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/changelog"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/notify"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/service/rbac"
)

const (
	teacherRoleSlug = "teacher"
	studentRoleSlug = "student"
)

var (
	ErrAlreadyPending  = errors.New("teacher_request: у пользователя уже есть активная заявка")
	ErrRequestNotFound = errors.New("teacher_request: заявка не найдена")
	ErrNotPending      = errors.New("teacher_request: заявка уже рассмотрена")
	ErrReasonRequired  = errors.New("teacher_request: причина отказа обязательна")
	// ErrHasOpenDebts — нельзя стать преподавателем, не закрыв долги.
	// Проверяется при подаче заявки и повторно при одобрении (за время
	// рассмотрения у студента мог появиться новый долг).
	ErrHasOpenDebts = errors.New("teacher_request: у пользователя есть незакрытые долги")
)

type Service struct {
	store     *repo.Store
	rbac      *rbac.Service
	notify    *notify.Service    // опционально — уведомление автору о решении
	changelog *changelog.Service // опционально — история смены роли при одобрении
}

// New собирает Service. notifySvc может быть nil — тогда уведомления о
// решении по заявке не шлются (удобно в юнит-тестах).
func New(store *repo.Store, rbacSvc *rbac.Service, notifySvc *notify.Service) *Service {
	return &Service{store: store, rbac: rbacSvc, notify: notifySvc}
}

// SetChangelog подключает запись смены роли (student → teacher) при одобрении
// заявки в change_logs — чтобы она была видна в /api/changelog и
// /api/users/{id}/story наравне с ручной сменой роли через rbac.ChangeRole,
// и её можно было откатить через /api/changelog/{id}/restore.
func (s *Service) SetChangelog(c *changelog.Service) { s.changelog = c }

// notifyDecision шлёт автору заявки уведомление о решении деканата.
// Best-effort: ошибка только логируется и не откатывает уже совершённую
// смену роли. reason для approved опционален, для rejected — обязателен.
func (s *Service) notifyDecision(ctx context.Context, userID pgtype.UUID, kind string, reason *string) {
	if s.notify == nil {
		return
	}
	payload := map[string]any{}
	if reason != nil && *reason != "" {
		payload["decision_reason"] = *reason
	}
	if err := s.notify.Notify(ctx, notify.Event{
		UserID:  pgutil.UUID(userID),
		Kind:    kind,
		Payload: payload,
	}); err != nil {
		slog.Warn("teacher_request: уведомление не отправлено",
			"kind", kind, "user_id", pgutil.UUID(userID).String(), "err", err)
	}
}

// Create подаёт заявку от имени actorID. Один pending на пользователя.
// Заблокировано, если у пользователя есть незакрытые долги — сперва нужно
// закрыть задолженности, и только потом переходить в преподаватели.
func (s *Service) Create(ctx context.Context, actorID uuid.UUID, reason *string) (queries.TeacherRoleRequest, error) {
	if err := s.ensureNoOpenDebts(ctx, actorID); err != nil {
		return queries.TeacherRoleRequest{}, err
	}

	existing, err := s.store.GetPendingTeacherRoleRequestForUser(ctx, pgutil.PgUUID(actorID))
	if err != nil && !repo.IsNotFound(err) {
		return queries.TeacherRoleRequest{}, fmt.Errorf("check pending: %w", err)
	}
	if err == nil && existing.Status == "pending" {
		return queries.TeacherRoleRequest{}, ErrAlreadyPending
	}

	req, err := s.store.CreateTeacherRoleRequest(ctx, queries.CreateTeacherRoleRequestParams{
		RequestedBy: pgutil.PgUUID(actorID),
		Reason:      reason,
	})
	if err != nil {
		return queries.TeacherRoleRequest{}, fmt.Errorf("create request: %w", err)
	}
	return req, nil
}

// ListPending возвращает заявки со статусом pending для деканата.
func (s *Service) ListPending(ctx context.Context, limit, offset int32) ([]queries.TeacherRoleRequest, error) {
	reqs, err := s.store.ListPendingTeacherRoleRequests(ctx, queries.ListPendingTeacherRoleRequestsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	return reqs, nil
}

// ListForUser возвращает историю заявок конкретного пользователя.
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]queries.TeacherRoleRequest, error) {
	reqs, err := s.store.ListTeacherRoleRequestsForUser(ctx, queries.ListTeacherRoleRequestsForUserParams{
		RequestedBy: pgutil.PgUUID(userID),
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list for user: %w", err)
	}
	return reqs, nil
}

// Approve одобряет заявку: в одной транзакции меняет статус и выдаёт роль teacher.
// actorID должен иметь roles.assign (проверяется через rbac.AssignRole).
func (s *Service) Approve(ctx context.Context, actorID, requestID uuid.UUID, reason *string) (queries.TeacherRoleRequest, error) {
	req, err := s.store.GetTeacherRoleRequestByID(ctx, pgutil.PgUUID(requestID))
	if err != nil {
		if repo.IsNotFound(err) {
			return queries.TeacherRoleRequest{}, ErrRequestNotFound
		}
		return queries.TeacherRoleRequest{}, fmt.Errorf("get request: %w", err)
	}
	if req.Status != "pending" {
		return queries.TeacherRoleRequest{}, ErrNotPending
	}

	targetID := pgutil.UUID(req.RequestedBy)

	// Повторная проверка долгов на момент одобрения: за время рассмотрения
	// заявки студенту мог быть выставлен новый долг.
	if err := s.ensureNoOpenDebts(ctx, targetID); err != nil {
		return queries.TeacherRoleRequest{}, err
	}

	var approved queries.TeacherRoleRequest
	if err := s.store.RunInTx(ctx, func(q *queries.Queries) error {
		var txErr error
		approved, txErr = q.ApproveTeacherRoleRequest(ctx, queries.ApproveTeacherRoleRequestParams{
			ReviewedBy:     pgutil.PgUUID(actorID),
			DecisionReason: reason,
			ID:             pgutil.PgUUID(requestID),
		})
		if txErr != nil {
			return fmt.Errorf("approve request: %w", txErr)
		}

		role, txErr := q.GetRoleBySlug(ctx, teacherRoleSlug)
		if txErr != nil {
			return fmt.Errorf("get teacher role: %w", txErr)
		}

		if _, txErr = q.AttachRoleToUser(ctx, queries.AttachRoleToUserParams{
			UserID:    pgutil.PgUUID(targetID),
			RoleID:    role.ID,
			CreatedBy: pgutil.PgUUID(actorID),
		}); txErr != nil {
			return fmt.Errorf("attach role: %w", txErr)
		}

		// Снимаем роль student: пользователь «переходит» в преподаватели,
		// а не накапливает роли. Это убирает протекание прошлых
		// студенческих пересдач/ведомостей в кабинет преподавателя.
		// Снимаем только если роль реально была — иначе DetachRoleFromUser
		// будет no-op, но в changelog ушло бы неверное "before: student".
		studentRole, txErr := q.GetRoleBySlug(ctx, studentRoleSlug)
		if txErr != nil {
			return fmt.Errorf("get student role: %w", txErr)
		}
		hadStudentRole, txErr := q.HasRole(ctx, queries.HasRoleParams{UserID: pgutil.PgUUID(targetID), Lower: studentRole.Slug})
		if txErr != nil {
			return fmt.Errorf("check student role: %w", txErr)
		}
		fromSlug, fromName := "", ""
		if hadStudentRole {
			fromSlug, fromName = studentRole.Slug, studentRole.Name
			if txErr = q.DetachRoleFromUser(ctx, queries.DetachRoleFromUserParams{
				UserID:    pgutil.PgUUID(targetID),
				RoleID:    studentRole.ID,
				DeletedBy: pgutil.PgUUID(actorID),
			}); txErr != nil {
				return fmt.Errorf("detach student role: %w", txErr)
			}
		}

		reqIDStr := requestID.String()
		_, txErr = q.CreateAuditEntry(ctx, queries.CreateAuditEntryParams{
			ActorID:    pgutil.PgUUID(actorID),
			Action:     "teacher_request.approve",
			TargetType: "teacher_role_request",
			TargetID:   &reqIDStr,
			Details:    auditDetails(map[string]any{"role": teacherRoleSlug}),
		})
		if txErr != nil {
			return txErr
		}

		// Та же смена роли, что и при ручном /api/users/{id}/roles/change —
		// одна запись "Роль изменена: Студент → Преподаватель" в истории
		// пользователя, доступная для просмотра и отката.
		if s.changelog != nil {
			targetStr := targetID.String()
			return s.changelog.LogRoleChangedTx(ctx, q, targetStr, fromSlug, fromName, role.Slug, role.Name, actorID)
		}
		return nil
	}); err != nil {
		return queries.TeacherRoleRequest{}, err
	}

	s.notifyDecision(ctx, req.RequestedBy, notify.KindTeacherRequestApproved, reason)
	return approved, nil
}

// Reject отклоняет заявку. reason обязателен.
func (s *Service) Reject(ctx context.Context, actorID, requestID uuid.UUID, reason string) (queries.TeacherRoleRequest, error) {
	if reason == "" {
		return queries.TeacherRoleRequest{}, ErrReasonRequired
	}

	req, err := s.store.GetTeacherRoleRequestByID(ctx, pgutil.PgUUID(requestID))
	if err != nil {
		if repo.IsNotFound(err) {
			return queries.TeacherRoleRequest{}, ErrRequestNotFound
		}
		return queries.TeacherRoleRequest{}, fmt.Errorf("get request: %w", err)
	}
	if req.Status != "pending" {
		return queries.TeacherRoleRequest{}, ErrNotPending
	}

	var rejected queries.TeacherRoleRequest
	if err := s.store.RunInTx(ctx, func(q *queries.Queries) error {
		var txErr error
		rejected, txErr = q.RejectTeacherRoleRequest(ctx, queries.RejectTeacherRoleRequestParams{
			ReviewedBy:     pgutil.PgUUID(actorID),
			DecisionReason: reason,
			ID:             pgutil.PgUUID(requestID),
		})
		if txErr != nil {
			return fmt.Errorf("reject request: %w", txErr)
		}

		reqIDStr := requestID.String()
		_, txErr = q.CreateAuditEntry(ctx, queries.CreateAuditEntryParams{
			ActorID:    pgutil.PgUUID(actorID),
			Action:     "teacher_request.reject",
			TargetType: "teacher_role_request",
			TargetID:   &reqIDStr,
			Details:    auditDetails(map[string]any{"reason": reason}),
		})
		return txErr
	}); err != nil {
		return queries.TeacherRoleRequest{}, err
	}

	s.notifyDecision(ctx, req.RequestedBy, notify.KindTeacherRequestRejected, &reason)
	return rejected, nil
}

// ensureNoOpenDebts возвращает ErrHasOpenDebts, если у пользователя есть
// хотя бы один долг в статусе open. Используется при подаче и одобрении
// заявки на роль преподавателя.
func (s *Service) ensureNoOpenDebts(ctx context.Context, userID uuid.UUID) error {
	n, err := s.store.CountOpenDebtsForStudent(ctx, pgutil.PgUUID(userID))
	if err != nil {
		return fmt.Errorf("count open debts: %w", err)
	}
	if n > 0 {
		return ErrHasOpenDebts
	}
	return nil
}

// auditDetails сериализует payload в JSON для audit_log.details.
// При ошибке маршалинга возвращает "{}" и логирует — лучше потерять
// детали записи, чем получить невалидный JSON в БД (или повредить
// аудит-журнал инъекцией кавычек из user input в reason).
func auditDetails(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("teacher_request: marshal audit details", "err", err)
		return []byte("{}")
	}
	return b
}
