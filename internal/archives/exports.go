package archives

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
)

// Export contains exactly one current-authorised native projection. Separate
// owners keep private assessment authorship, suppression and evidence policy;
// an archive membership grants none of those capabilities by itself.
type Export struct {
	EventID         uuid.UUID                   `json:"event_id"`
	EventRevision   int64                       `json:"event_revision"`
	SourceState     string                      `json:"source_state"`
	Source          string                      `json:"source"`
	ResourceID      *uuid.UUID                  `json:"resource_id,omitempty"`
	Roster          *attendance.OperationalPage `json:"roster,omitempty"`
	Submission      *submissions.Page           `json:"submission,omitempty"`
	AssessmentPanel *scorecards.PanelReport     `json:"assessment_panel,omitempty"`
	Assessments     *scorecards.Page            `json:"assessments,omitempty"`
	Projects        *outcomes.ProjectPage       `json:"projects,omitempty"`
	Feedback        *surveys.Report             `json:"feedback,omitempty"`
	Observations    *outcomes.ObservationPage   `json:"observations,omitempty"`
}

type ExportRequest struct {
	Source     string
	ContextID  uuid.UUID
	ResourceID uuid.UUID
	Cursor     string
	Limit      int32
	Audience   string
}

func (s Service) Export(
	ctx context.Context,
	actor, eventID uuid.UUID,
	r ExportRequest,
) (Export, error) {
	out := Export{EventID: eventID, Source: r.Source, SourceState: "available"}
	if err := r.validate(); err != nil {
		return out, err
	}
	after := uuid.Nil
	if r.Cursor != "" && r.Source != "objective-evidence" {
		var err error
		after, err = uuid.Parse(r.Cursor)
		if err != nil {
			return out, access.ErrInvalid
		}
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		e, err := archived(ctx, q, actor, eventID, access.EventRead)
		if err != nil {
			return wrap("selected source "+r.Source, err)
		}
		out.EventRevision = e.Revision
		out.SourceState, err = s.exportSourceState(ctx, tx, q, e, actor, r.Source)
		if err != nil {
			return err
		}
		count := 0
		if out.SourceState == "available" {
			switch r.Source {
			case "roster":
				v, err := (attendance.Service{Pool: s.Pool, Now: s.Clock}).LogisticsInTransaction(
					ctx,
					tx,
					actor,
					eventID,
					after,
					r.Limit,
					true,
				)
				if err != nil {
					return wrap("selected source "+r.Source, err)
				}
				out.Roster = &v
				count = len(v.Items)
			case "deliverables":
				v, err := (submissions.Service{Pool: s.Pool, Clock: s.Clock}).Report(
					ctx,
					tx,
					actor,
					eventID,
					r.ResourceID,
					r.Audience,
				)
				if err != nil {
					return wrap("selected source "+r.Source, err)
				}
				out.Submission = &v
				out.ResourceID = &r.ResourceID
				count = 1
			case "assessments":
				owner := s.scorecardsOwner()
				if r.ContextID != uuid.Nil {
					panel, err := owner.PanelReportInTransaction(
						ctx,
						tx,
						actor,
						eventID,
						r.ResourceID,
						r.ContextID,
					)
					if err != nil {
						return wrap("export selected recorded assessment context", err)
					}
					out.AssessmentPanel = &panel
					out.ResourceID = &r.ResourceID
					count = len(panel.Cards)
					break
				}
				v, err := owner.ReadInTransaction(
					ctx,
					tx,
					actor,
					eventID,
					r.ResourceID,
					after,
					int(r.Limit),
				)
				if err != nil {
					return wrap("selected source "+r.Source, err)
				}
				out.Assessments = &v
				out.ResourceID = &r.ResourceID
				count = len(v.Items)
			case "published-outcomes":
				owner := s.outcomesOwner()
				v, err := owner.PublishedEventProjectsInTransaction(
					ctx,
					tx,
					actor,
					eventID,
					after,
					r.Limit,
				)
				if err != nil {
					return wrap("selected source "+r.Source, err)
				}
				out.Projects = &v
				count = len(v.Items)
			case "feedback":
				owner := s.surveysOwner()
				v, err := owner.PublishedReportInTransaction(ctx, tx, actor, eventID, r.ResourceID)
				if err != nil {
					return wrap("selected source "+r.Source, err)
				}
				out.Feedback = &v
				out.ResourceID = &r.ResourceID
				count = 1
			case "objective-evidence":
				owner := s.outcomesOwner()
				v, err := owner.ObservationsInTransaction(
					ctx,
					tx,
					actor,
					eventID,
					r.Cursor,
					r.Limit,
				)
				if err != nil {
					return wrap("selected source "+r.Source, err)
				}
				out.Observations = &v
				count = len(v.Items)
			default:
				return access.ErrInvalid
			}
		}
		reference := r.ResourceID
		if r.ContextID != uuid.Nil {
			reference = r.ContextID
		}
		if reference == uuid.Nil {
			reference = eventID
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   audit.ArchiveExported,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{
					Count:       &count,
					Revision:    &e.Revision,
					ReferenceID: &reference,
				},
			},
		)
	})
	if err != nil {
		return Export{}, fmt.Errorf("archive selected-source export: %w", err)
	}
	return out, nil
}

func (s Service) scorecardsOwner() scorecards.Service {
	owner := s.Scorecards
	if owner.Pool == nil {
		owner.Pool = s.Pool
	}
	if owner.Now == nil {
		owner.Now = s.Clock
	}
	return owner
}

func (s Service) outcomesOwner() outcomes.Service {
	owner := s.Outcomes
	if owner.Pool == nil {
		owner.Pool = s.Pool
	}
	if owner.Now == nil {
		owner.Now = s.Clock
	}
	return owner
}

func (s Service) surveysOwner() surveys.Service {
	owner := s.Surveys
	if owner.Pool == nil {
		owner.Pool = s.Pool
	}
	if owner.Now == nil {
		owner.Now = s.Clock
	}
	return owner
}

// exportSourceState preserves native field authority before exposing even an
// expiry indicator; published suppressed feedback is retained independently.
func (s Service) exportSourceState(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	e dbgen.Event,
	actor uuid.UUID,
	source string,
) (string, error) {
	var action access.Action
	switch source {
	case "roster":
		action = access.ProfileExport
	case "objective-evidence":
		action = access.StaffRead
	case "published-outcomes":
		action = access.EventRead
	default:
		return "available", nil
	}
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		e.ID,
		e.ID,
		action,
	); err != nil {
		return "", wrap("authorize retained export source", err)
	}
	state, err := privacy.OperationalFieldsStateInTransaction(ctx, tx, e, s.now())
	return state, wrap("check retained export source", err)
}

func (r ExportRequest) validate() error {
	if r.Limit < 1 || r.Limit > 100 {
		return access.ErrInvalid
	}
	if r.ContextID != uuid.Nil && (r.Source != "assessments" || r.Cursor != "") {
		return access.ErrInvalid
	}
	if r.Audience != "" && r.Source != "deliverables" {
		return access.ErrInvalid
	}
	switch r.Source {
	case "deliverables":
		if r.ResourceID == uuid.Nil || r.Cursor != "" {
			return access.ErrInvalid
		}
	case "assessments":
		if r.ResourceID == uuid.Nil {
			return access.ErrInvalid
		}
	case "feedback":
		if r.ResourceID == uuid.Nil || r.Cursor != "" {
			return access.ErrInvalid
		}
	case "roster", "published-outcomes", "objective-evidence":
		if r.ResourceID != uuid.Nil {
			return access.ErrInvalid
		}
	default:
		return access.ErrInvalid
	}
	return nil
}
