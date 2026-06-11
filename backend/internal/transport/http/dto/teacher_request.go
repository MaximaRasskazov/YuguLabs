package dto

import (
	"time"

	"github.com/google/uuid"

	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/pgutil"
	"github.com/MaximaRasskazov/Academic-debt-system/backend/internal/repo/queries"
)

// TeacherRequestCreateRequest — тело POST /api/teacher-requests.
type TeacherRequestCreateRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// TeacherRequestRejectRequest — тело POST /api/teacher-requests/:id/reject.
type TeacherRequestRejectRequest struct {
	Reason string `json:"reason"`
}

// TeacherRequestApproveRequest — тело POST /api/teacher-requests/:id/approve.
// reason опционален при одобрении.
type TeacherRequestApproveRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// TeacherRequestResponse — публичное представление заявки.
type TeacherRequestResponse struct {
	ID             uuid.UUID  `json:"id"`
	RequestedBy    uuid.UUID  `json:"requested_by"`
	Reason         *string    `json:"reason,omitempty"`
	Status         string     `json:"status"`
	ReviewedBy     *uuid.UUID `json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	DecisionReason *string    `json:"decision_reason,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// FromTeacherRequest маппит sqlc-структуру в ответ API.
func FromTeacherRequest(r queries.TeacherRoleRequest) TeacherRequestResponse {
	out := TeacherRequestResponse{
		ID:             pgutil.UUID(r.ID),
		RequestedBy:    pgutil.UUID(r.RequestedBy),
		Reason:         r.Reason,
		Status:         r.Status,
		DecisionReason: r.DecisionReason,
		CreatedAt:      r.CreatedAt.Time,
	}
	if r.ReviewedBy.Valid {
		id := pgutil.UUID(r.ReviewedBy)
		out.ReviewedBy = &id
	}
	if r.ReviewedAt.Valid {
		t := r.ReviewedAt.Time
		out.ReviewedAt = &t
	}
	return out
}

// FromTeacherRequests маппит срез заявок.
func FromTeacherRequests(rs []queries.TeacherRoleRequest) []TeacherRequestResponse {
	out := make([]TeacherRequestResponse, 0, len(rs))
	for _, r := range rs {
		out = append(out, FromTeacherRequest(r))
	}
	return out
}

// AssignRoleRequest — тело POST /api/users/:id/roles.
type AssignRoleRequest struct {
	RoleSlug string `json:"role_slug"`
}

// ChangeRoleRequest — тело POST /api/users/:id/roles/change: атомарная
// замена роли. FromSlug может быть пустым (у пользователя не было роли).
type ChangeRoleRequest struct {
	FromSlug string `json:"from_slug"`
	ToSlug   string `json:"to_slug"`
}

// PermissionResponse — представление permission для API.
type PermissionResponse struct {
	ID          uuid.UUID `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
}

// FromPermission маппит sqlc-permission в ответ API.
func FromPermission(p queries.Permission) PermissionResponse {
	return PermissionResponse{
		ID:          pgutil.UUID(p.ID),
		Slug:        p.Slug,
		Name:        p.Name,
		Description: p.Description,
	}
}

// FromPermissions маппит срез permissions.
func FromPermissions(ps []queries.Permission) []PermissionResponse {
	out := make([]PermissionResponse, 0, len(ps))
	for _, p := range ps {
		out = append(out, FromPermission(p))
	}
	return out
}
