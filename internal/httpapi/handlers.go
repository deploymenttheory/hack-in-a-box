package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/credentials"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubidentity"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/preparation"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamsworkflow"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

type Dependencies struct {
	TeamsWorkflow  *teamsworkflow.Service
	SMTP           *smtpintegration.Service
	MailContacts   mailcontact.Service
	Scorecards     scorecards.Service
	Rubrics        rubric.Service
	Inbox          inbox.Service
	Help           help.Service
	Presentation   presentation.Service
	Communications communications.Service
	Credentials    credentials.Service
	Mentors        mentors.Service
	Observations   observations.Service
	Content        content.Service
	Submissions    submissions.Service
	Attendance     attendance.Service
	Preparation    preparation.Service
	Teams          teams.Service
	GitHub         *githubidentity.Service
	Pool           *pgxpool.Pool
	Auth           *identity.Auth
	Audit          audit.Service
	Files          files.Service
	Permissions    permissions.Service
	Events         event.Service
	Privacy        privacy.Service
	Templates      templates.Service
	Registration   registration.Service
}
type handlers struct{ Dependencies }

var _ contract.StrictServerInterface = handlers{}

func (h handlers) GetHealth(
	context.Context,
	contract.GetHealthRequestObject,
) (contract.GetHealthResponseObject, error) {
	return contract.GetHealth200JSONResponse{Status: contract.Ok}, nil
}

func (h handlers) GetReady(
	ctx context.Context,
	_ contract.GetReadyRequestObject,
) (contract.GetReadyResponseObject, error) {
	if h.Pool == nil || h.Files.Storage == nil {
		return nil, database.ErrDatabase
	}
	if err := h.Pool.Ping(ctx); err != nil {
		return nil, database.ErrDatabase
	}
	if err := database.CheckSchema(ctx, h.Pool); err != nil {
		return nil, database.ErrSchema
	}
	if err := h.Files.Storage.Ready(ctx); err != nil {
		return nil, database.ErrDatabase
	}
	return contract.GetReady200JSONResponse{Status: contract.Ok}, nil
}

func (h handlers) GetSession(
	ctx context.Context,
	_ contract.GetSessionRequestObject,
) (contract.GetSessionResponseObject, error) {
	person := actor(ctx)
	response := contract.GetSession200JSONResponse{
		CsrfToken:  h.Auth.Sessions.GetString(ctx, "csrf"),
		DraftScope: h.Auth.DraftScope(ctx),
	}
	response.Person.Id = person.ID
	response.Person.DisplayName = person.DisplayName
	response.Person.PlatformAdmin = person.PlatformAdmin
	allowed, err := dbgen.New(h.Auth.Pool).CanCreateEvent(ctx, person.ID)
	if err != nil {
		return nil, fmt.Errorf("check creation capability: %w", err)
	}
	response.CanCreateEvents = allowed
	return response, nil
}

func (h handlers) Logout(
	ctx context.Context,
	_ contract.LogoutRequestObject,
) (contract.LogoutResponseObject, error) {
	if err := h.Auth.Logout(ctx); err != nil {
		return nil, fmt.Errorf("logout: %w", err)
	}
	return contract.Logout204Response{}, nil
}

func page(cursor *int64, limit *int) (int64, int32) {
	start := int64(0)
	size := int32(50)
	if cursor != nil {
		start = *cursor
	}
	if limit != nil && *limit >= 1 && *limit <= 100 {
		size = int32(*limit)
	}
	return start, size
}

func (h handlers) ListAudit(
	ctx context.Context,
	r contract.ListAuditRequestObject,
) (contract.ListAuditResponseObject, error) {
	cursor, limit := page(r.Params.Cursor, r.Params.Limit)
	records, err := h.Audit.List(ctx, actor(ctx).ID, r.EventId, cursor, limit, false)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	result := contract.AuditPage{
		Items:      make([]contract.AuditEvent, 0, len(records)),
		NextCursor: cursor,
	}
	for _, record := range records {
		entry := contract.AuditEvent{
			Id:         record.ID,
			ActorId:    record.ActorID,
			Action:     record.Action,
			TargetId:   record.TargetID,
			Sequence:   record.Sequence,
			ReasonCode: record.ReasonCode,
			OccurredAt: record.OccurredAt,
		}
		if err := json.Unmarshal(record.Metadata, &entry.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		result.Items = append(result.Items, entry)
		result.NextCursor = record.Sequence
	}
	return contract.ListAudit200JSONResponse(result), nil
}

func (h handlers) ExportAudit(
	ctx context.Context,
	r contract.ExportAuditRequestObject,
) (contract.ExportAuditResponseObject, error) {
	cursor, limit := page(r.Params.Cursor, r.Params.Limit)
	records, err := h.Audit.List(ctx, actor(ctx).ID, r.EventId, cursor, limit, true)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	result := contract.AuditPage{
		Items:      make([]contract.AuditEvent, 0, len(records)),
		NextCursor: cursor,
	}
	for _, record := range records {
		entry := contract.AuditEvent{
			Id:         record.ID,
			ActorId:    record.ActorID,
			Action:     record.Action,
			TargetId:   record.TargetID,
			Sequence:   record.Sequence,
			ReasonCode: record.ReasonCode,
			OccurredAt: record.OccurredAt,
		}
		if err := json.Unmarshal(record.Metadata, &entry.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		result.Items = append(result.Items, entry)
		result.NextCursor = record.Sequence
	}
	disposition := "attachment; filename=audit.json"
	return contract.ExportAudit200JSONResponse{
		Body:    result,
		Headers: contract.ExportAudit200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

func (h handlers) UploadFile(
	ctx context.Context,
	r contract.UploadFileRequestObject,
) (contract.UploadFileResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	revision, err := strconv.ParseInt(strings.Trim(r.Params.IfMatch, "\""), 10, 64)
	if err != nil {
		return nil, access.ErrInvalid
	}
	file, err := h.Files.Upload(
		ctx,
		files.Upload{
			Actor:     actor(ctx).ID,
			TeamID:    r.Body.TeamId,
			Reason:    valueOrEmpty(r.Body.Reason),
			Event:     r.EventId,
			Key:       r.Params.IdempotencyKey,
			Revision:  revision,
			Filename:  r.Body.Filename,
			MediaType: string(r.Body.MediaType),
			Content:   r.Body.Content,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	return contract.UploadFile201JSONResponse{
		Id:        file.ID,
		EventId:   file.EventID,
		OwnerId:   file.OwnerID,
		Filename:  file.Filename,
		MediaType: file.MediaType,
		Checksum:  file.Checksum,
		SizeBytes: file.SizeBytes,
		CreatedAt: file.CreatedAt,
	}, nil
}

func (h handlers) DownloadFile(
	ctx context.Context,
	r contract.DownloadFileRequestObject,
) (contract.DownloadFileResponseObject, error) {
	file, data, err := h.Files.Download(ctx, actor(ctx).ID, r.EventId, r.FileId)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename})
	return contract.DownloadFile200ApplicationoctetStreamResponse{
		Body:          bytes.NewReader(data),
		ContentLength: file.SizeBytes,
		Headers:       contract.DownloadFile200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

func respondError(w http.ResponseWriter, r *http.Request, err error) {
	var formError *forms.Invalid
	if errors.As(err, &formError) {
		body := errorBody(r, "form_invalid", "Resolve the form fields before continuing.")
		items := make([]contract.Blocker, 0, len(formError.Issues))
		for _, item := range formError.Issues {
			items = append(
				items,
				contract.Blocker{Field: item.Field, Code: item.Code, Message: item.Message},
			)
		}
		body.Error.Blockers = &items
		writeJSON(w, 400, body)
		return
	}
	var blocked *event.Blocked
	if errors.As(err, &blocked) {
		body := errorBody(r, "event_blocked", "Resolve the event blockers before continuing.")
		items := make([]contract.Blocker, 0, len(blocked.Issues))
		for _, item := range blocked.Issues {
			items = append(
				items,
				contract.Blocker{Field: item.Field, Code: item.Code, Message: item.Message},
			)
		}
		body.Error.Blockers = &items
		writeJSON(w, 409, body)
		return
	}
	if status, code, message := emailError(err); status != 0 {
		writeError(w, r, status, code, message)
		return
	}
	if code, message := assignmentConflict(err); code != "" {
		writeError(w, r, 409, code, message)
		return
	}
	status, code, message := 500, "internal_error", "An internal error occurred."
	switch {
	case errors.Is(err, registration.ErrClosed):
		status, code, message = 409, "registration_closed", registration.ErrClosed.Error()
	case errors.Is(err, registration.ErrDuplicate):
		status, code, message = 409, "registration_exists", registration.ErrDuplicate.Error()
	case errors.Is(err, registration.ErrCapacity):
		status, code, message = 409, "capacity_unavailable", registration.ErrCapacity.Error()
	case errors.Is(err, permissions.ErrWithdrawal):
		status, code, message = 409, "withdrawal_required", permissions.ErrWithdrawal.Error()
	case errors.Is(err, permissions.ErrIneligible):
		status, code, message = 409, "competition_ineligible", permissions.ErrIneligible.Error()
	case errors.Is(err, permissions.ErrAssessment):
		status, code, message = 409, "assessment_conflict", permissions.ErrAssessment.Error()
	case errors.Is(err, permissions.ErrLastOrganiser):
		status, code, message = 409, "last_organiser", permissions.ErrLastOrganiser.Error()
	case errors.Is(err, access.ErrUnauthenticated):
		status, code, message = 401, "unauthenticated", "Sign in to continue."
	case errors.Is(err, access.ErrForbidden):
		status, code, message = 403, "forbidden", "You do not have access to this resource."
	case errors.Is(err, identity.ErrCSRF):
		status, code, message = 403, "csrf_invalid", "Refresh the page and try again."
	case errors.Is(err, access.ErrNotFound):
		status, code, message = 404, "not_found", "The requested resource was not found."
	case errors.Is(err, access.ErrConflict):
		status, code, message = 409, "revision_conflict", "This resource has changed. Refresh and try again."
	case errors.Is(err, files.ErrIdempotency):
		status, code, message = 409, "idempotency_conflict", "This action key has already been used for different content."
	case errors.Is(err, access.ErrInvalid), errors.Is(err, files.ErrPath):
		status, code, message = 400, "invalid_request", "Check the request fields and try again."
	case errors.Is(err, files.ErrSize):
		status, code, message = 413, "body_too_large", "The file exceeds the upload limit."
	case errors.Is(err, files.ErrQuota):
		status, code, message = 413, "quota_exceeded", "This event has reached its file quota."
	case errors.Is(err, files.ErrMedia):
		status, code, message = 415, "unsupported_media", "The file type is unsupported or does not match its content."
	case errors.Is(err, files.ErrMissing):
		status, code, message = 503, "object_missing", "The stored file is unavailable. Ask an administrator to check the file volume."
	case errors.Is(err, secretbox.ErrUnavailable),
		errors.Is(err, database.ErrDatabase),
		errors.Is(err, database.ErrSchema):
		status, code, message = 503, "unavailable", "A required service is temporarily unavailable."
	case errors.Is(err, githubidentity.ErrLink):
		status, code, message = 400, "github_link_failed", githubidentity.ErrLink.Error()
	case errors.Is(err, githubidentity.ErrDisabled):
		status, code, message = 503, "github_unconfigured", githubidentity.ErrDisabled.Error()
	case errors.Is(err, identity.ErrLogin):
		status, code, message = 401, "login_failed", "Sign-in could not be completed. Start sign-in again."
	}
	writeError(w, r, status, code, message)
}

func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
