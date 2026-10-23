package surveys

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// Dashboard is an explicitly organiser-restricted administrative count. It is
// never the public report denominator or an independently demonstrated outcome.
type Dashboard struct {
	CanReviewReport            bool          `json:"can_review_report"`
	EventRevision              int64         `json:"event_revision"`
	Timezone                   string        `json:"timezone"`
	Configuration              Configuration `json:"configuration"`
	CanManage                  bool          `json:"can_manage"`
	RetainedInvitations        int           `json:"retained_invitations"`
	CurrentEligibleInvitations int           `json:"current_eligible_invitations"`
	SubmittedResponses         int           `json:"submitted_responses"`
	CurrentEligibleResponses   int           `json:"current_eligible_responses"`
	ObservedAt                 time.Time     `json:"observed_at"`
	ResponseVisibility         string        `json:"response_visibility"`
}

func (s Service) Dashboard(ctx context.Context, actor, eventID, id uuid.UUID) (Dashboard, error) {
	var out Dashboard
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock survey dashboard: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		if err = s.privateCurrent(ctx, q, e); err != nil {
			return err
		}
		_, cfg, err := current(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		var dates event.Configuration
		if err = json.Unmarshal(e.Configuration, &dates); err != nil {
			return fmt.Errorf("read dashboard event policy: %w", err)
		}
		invites, err := q.SurveyInvitations(
			ctx,
			dbgen.SurveyInvitationsParams{EventID: eventID, SurveyID: id},
		)
		if err != nil {
			return fmt.Errorf("read retained invitations: %w", err)
		}
		responses, err := q.SurveyResponseHeadsPage(
			ctx,
			dbgen.SurveyResponseHeadsPageParams{
				EventID:     eventID,
				SurveyID:    id,
				AfterPerson: uuid.Nil,
				PageSize:    10001,
			},
		)
		if err != nil {
			return fmt.Errorf("read submitted response counts: %w", err)
		}
		if len(invites) > 10000 || len(responses) > 10000 {
			return access.ErrInvalid
		}
		current, err := s.people(ctx, q, eventID, uuid.Nil, cfg.Definition.Form.Audience)
		if err != nil {
			return err
		}
		eligible := map[uuid.UUID]bool{}
		for _, person := range current {
			eligible[person] = true
		}
		out = Dashboard{
			Configuration:       cfg,
			CanManage:           e.WritesAllowed,
			RetainedInvitations: len(invites),
			SubmittedResponses:  len(responses),
			ObservedAt:          s.now(),
			ResponseVisibility:  ResponseVisibility,
		}
		out.CanReviewReport = e.WritesAllowed && cfg.PublishedAt != nil &&
			!s.now().Before(cfg.Definition.ClosesAt) &&
			!s.now().Before(dates.LiveEnd)
		out.Timezone = dates.Timezone
		out.EventRevision = e.Revision

		for _, invite := range invites {
			if eligible[invite.PersonID] {
				out.CurrentEligibleInvitations++
			}
		}
		for _, response := range responses {
			if eligible[response.PersonID] {
				out.CurrentEligibleResponses++
			}
		}
		return nil
	})
	return out, wrap("read restricted survey dashboard", err)
}

// ExportResponses is a deliberate audited read using current organiser and
// privacy authority. It exports no hidden answers, mailbox or unrelated profile.
func (s Service) ExportResponses(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) ([]byte, error) {
	var out []byte
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock restricted response export: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		if err = s.privateCurrent(ctx, q, e); err != nil {
			return err
		}
		_, cfg, err := current(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		rows, err := q.CurrentSurveyResponses(
			ctx,
			dbgen.CurrentSurveyResponsesParams{EventID: eventID, SurveyID: id},
		)
		if err != nil {
			return fmt.Errorf("read restricted export responses: %w", err)
		}
		if len(rows) > 10000 {
			return access.ErrInvalid
		}
		var buffer bytes.Buffer
		writer := csv.NewWriter(&buffer)
		if err = writer.Write(
			[]string{
				"person_id",
				"display_name",
				"response_revision",
				"form_version",
				"submitted_at",
				"question_id",
				"question_label",
				"answer_state",
				"answer_json",
			},
		); err != nil {
			return fmt.Errorf("write export headings: %w", err)
		}
		authors, err := q.SurveyResponseAuthorNames(
			ctx,
			dbgen.SurveyResponseAuthorNamesParams{EventID: eventID, SurveyID: id},
		)
		if err != nil {
			return fmt.Errorf("read response author names: %w", err)
		}
		names := map[uuid.UUID]string{}
		for _, person := range authors {
			names[person.ID] = person.DisplayName
		}
		for _, row := range rows {
			response, readErr := decodeResponse(row)
			if readErr != nil {
				return readErr
			}
			for _, field := range cfg.Definition.Form.Fields {
				answer, present := response.Answers[field.ID.String()]
				state := "not_answered"
				if present {
					state = "answered"
				}
				record := []string{
					row.PersonID.String(),
					names[row.PersonID],
					strconv.FormatInt(response.Revision, 10),
					strconv.FormatInt(response.FormVersion, 10),
					response.SubmittedAt.Format(time.RFC3339Nano),
					field.ID.String(),
					field.Label,
					state,
					string(answer),
				}
				for i := range record {
					record[i] = csvLiteral(record[i])
				}
				if err = writer.Write(record); err != nil {
					return fmt.Errorf("write restricted response row: %w", err)
				}
			}
		}
		writer.Flush()
		if err = writer.Error(); err != nil {
			return fmt.Errorf("flush restricted response CSV: %w", err)
		}
		if err = record(
			ctx,
			q,
			actor,
			eventID,
			id,
			audit.SurveyResponsesExported,
			cfg.Version,
		); err != nil {
			return err
		}
		out = buffer.Bytes()
		return nil
	})
	return out, wrap("export restricted survey responses", err)
}

func csvLiteral(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") ||
		strings.HasPrefix(value, "\n") ||
		(len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + value
	}
	return value
}
