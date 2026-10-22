package communications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func authorize(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID, write bool) error {
	action := access.StaffRead
	if write {
		action = access.EventEdit
	}
	err := (access.EventPolicy{Queries: q}).Authorize(ctx, actor, eventID, eventID, action)
	return contextError("authorize", err)
}

func loadPage(ctx context.Context, q *dbgen.Queries, row dbgen.Event) (Page, error) {
	page := Page{
		Revision:  row.Revision,
		Templates: []Template{},
		Audiences: []Audience{},
		Variables: slices.Clone(variables),
	}
	versions, err := q.CommunicationVersions(ctx, row.ID)
	if err != nil {
		return page, contextError("loadPage", err)
	}
	if len(versions) > 1000 {
		return page, blocked("too_many_templates")
	}
	for _, v := range versions {
		if v.Kind == "template" {
			var t Template
			if err = json.Unmarshal(v.Document, &t); err != nil {
				return page, contextError("loadPage", err)
			}
			page.Templates = append(page.Templates, t)
		} else {
			var a Audience
			if err = json.Unmarshal(v.Document, &a); err != nil {
				return page, contextError("loadPage", err)
			}
			page.Audiences = append(page.Audiences, a)
		}
	}
	return page, nil
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return contextError("Read", err)
		}
		if err = authorize(ctx, q, actor, eventID, false); err != nil {
			return contextError("Read", err)
		}
		result, err = loadPage(ctx, q, row)
		return contextError("Read", err)
	})
	return result, contextError("Read", err)
}

func saveVersion(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, id uuid.UUID,
	kind string,
	expected int64,
	document any,
) error {
	current, err := q.CommunicationVersion(
		ctx,
		dbgen.CommunicationVersionParams{EventID: eventID, ID: id, Kind: kind},
	)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if expected != 0 {
			return access.ErrConflict
		}
	case err != nil:
		return contextError("saveVersion", err)
	case current.Version != expected:
		return access.ErrConflict
	}
	data, err := json.Marshal(document)
	if err != nil {
		return contextError("saveVersion", err)
	}
	err = q.SaveCommunicationVersion(
		ctx,
		dbgen.SaveCommunicationVersionParams{
			EventID:  eventID,
			ID:       id,
			Kind:     kind,
			Version:  expected + 1,
			Document: data,
			ActorID:  actor,
		},
	)
	return contextError("saveVersion", err)
}

func loadPair(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, templateID, audienceID uuid.UUID,
) (Template, Audience, error) {
	var t Template
	var a Audience
	tv, err := q.CommunicationVersion(
		ctx,
		dbgen.CommunicationVersionParams{EventID: eventID, ID: templateID, Kind: "template"},
	)
	if err != nil {
		return t, a, access.ErrNotFound
	}
	av, err := q.CommunicationVersion(
		ctx,
		dbgen.CommunicationVersionParams{EventID: eventID, ID: audienceID, Kind: "audience"},
	)
	if err != nil {
		return t, a, access.ErrNotFound
	}
	if err = json.Unmarshal(tv.Document, &t); err != nil {
		return t, a, contextError("loadPair", err)
	}
	err = json.Unmarshal(av.Document, &a)
	return t, a, contextError("loadPair", err)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	c Command,
) (Page, error) {
	var result Page
	if strings.TrimSpace(c.Reason) == "" || len(c.Reason) > 2000 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return contextError("Change", err)
		}
		if err = authorize(ctx, q, actor, eventID, true); err != nil {
			return contextError("Change", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		preview, err := s.apply(ctx, q, row, actor, c)
		if err != nil {
			return contextError("Change", err)
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: eventID,
				Action:   audit.CommunicationChanged,
				Reason:   audit.OrganiserDecision,
			},
		); err != nil {
			return contextError("Change", err)
		}
		row, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return contextError("Change", err)
		}
		result, err = loadPage(ctx, q, row)
		result.Preview = preview
		return contextError("Change", err)
	})
	if err != nil {
		return result, fmt.Errorf("change communications: %w", err)
	}
	return result, nil
}

func (s Service) apply(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Command,
) (*Preview, error) {
	switch c.Action {
	case "save_template":
		if c.Template == nil {
			return nil, access.ErrInvalid
		}
		t := *c.Template
		if err := validateTemplate(t); err != nil {
			return nil, contextError("apply", err)
		}
		expected := t.Version
		t.Version++
		return nil, saveVersion(ctx, q, actor, row.ID, t.ID, "template", expected, t)
	case "save_audience":
		if c.Audience == nil {
			return nil, access.ErrInvalid
		}
		a := *c.Audience
		if err := validateAudience(a); err != nil {
			return nil, contextError("apply", err)
		}
		expected := a.Version
		a.Version++
		if a.Roles == nil {
			a.Roles = []string{}
		}
		if a.RegistrationStates == nil {
			a.RegistrationStates = []string{}
		}
		if a.Teams == nil {
			a.Teams = []uuid.UUID{}
		}
		for _, id := range a.Teams {
			if _, err := q.CommunicationTeam(
				ctx,
				dbgen.CommunicationTeamParams{EventID: row.ID, ID: id},
			); err != nil {
				return nil, access.ErrInvalid
			}
		}
		return nil, saveVersion(ctx, q, actor, row.ID, a.ID, "audience", expected, a)
	case "preview":
		t, a, err := loadPair(ctx, q, row.ID, c.TemplateID, c.AudienceID)
		if err != nil {
			return nil, contextError("apply", err)
		}
		preview, err := s.build(ctx, q, row, t, a)
		if err != nil {
			return nil, contextError("apply", err)
		}
		data, err := json.Marshal(preview)
		if err != nil {
			return nil, contextError("apply", err)
		}
		err = q.SaveCommunicationPreview(
			ctx,
			dbgen.SaveCommunicationPreviewParams{
				EventID:         row.ID,
				ID:              preview.ID,
				TemplateID:      t.ID,
				TemplateVersion: t.Version,
				AudienceID:      a.ID,
				AudienceVersion: a.Version,
				ActorID:         actor,
				Snapshot:        data,
			},
		)
		return &preview, contextError("apply", err)
	case "approve":
		return s.approve(ctx, q, row, actor, c)
	default:
		return nil, access.ErrInvalid
	}
}

func previewEquivalent(a, b Preview) bool {
	a.ID = uuid.Nil
	b.ID = uuid.Nil
	a.Approved = false
	b.Approved = false
	return digest(a) == digest(b)
}

func (s Service) approve(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Command,
) (*Preview, error) {
	saved, err := q.CommunicationPreview(
		ctx,
		dbgen.CommunicationPreviewParams{EventID: row.ID, ID: c.PreviewID},
	)
	if err != nil {
		return nil, access.ErrNotFound
	}
	var old Preview
	if err = json.Unmarshal(saved.Snapshot, &old); err != nil {
		return nil, contextError("approve", err)
	}
	t, a, err := loadPair(ctx, q, row.ID, saved.TemplateID, saved.AudienceID)
	if err != nil {
		return nil, contextError("approve", err)
	}
	fresh, err := s.build(ctx, q, row, t, a)
	if err != nil {
		return nil, contextError("approve", err)
	}
	if len(fresh.Blockers) > 0 || !previewEquivalent(old, fresh) {
		return nil, blocked("message_review_required")
	}
	_, err = q.CommunicationApproval(
		ctx,
		dbgen.CommunicationApprovalParams{EventID: row.ID, PreviewID: saved.ID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		err = q.ApproveCommunication(
			ctx,
			dbgen.ApproveCommunicationParams{
				EventID:   row.ID,
				PreviewID: saved.ID,
				ActorID:   actor,
				Reason:    c.Reason,
			},
		)
	}
	if err != nil {
		return nil, contextError("approve", err)
	}
	old.Approved = true
	return &old, nil
}

// ResolveApprovedInTransaction is the execution boundary for campaign/delivery
// owners. They must call it in the transaction that records recipient attempts,
// and retain the resulting recipients with their eventual provider receipts.
// No addresses, credentials or private question/assessment prose are returned.
func (s Service) ResolveApprovedInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, previewID uuid.UUID,
) (Preview, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return Preview{}, contextError("ResolveApprovedInTransaction", err)
	}
	approval, err := q.CommunicationApproval(
		ctx,
		dbgen.CommunicationApprovalParams{EventID: eventID, PreviewID: previewID},
	)
	if err != nil {
		return Preview{}, access.ErrForbidden
	}
	if err = authorize(ctx, q, approval.ActorID, eventID, true); err != nil {
		return Preview{}, contextError("ResolveApprovedInTransaction", err)
	}
	saved, err := q.CommunicationPreview(
		ctx,
		dbgen.CommunicationPreviewParams{EventID: eventID, ID: previewID},
	)
	if err != nil {
		return Preview{}, contextError("ResolveApprovedInTransaction", err)
	}
	var old Preview
	if err = json.Unmarshal(saved.Snapshot, &old); err != nil {
		return Preview{}, contextError("ResolveApprovedInTransaction", err)
	}
	t, a, err := loadPair(ctx, q, eventID, saved.TemplateID, saved.AudienceID)
	if err != nil {
		return Preview{}, contextError("ResolveApprovedInTransaction", err)
	}
	if t.Version != saved.TemplateVersion || a.Version != saved.AudienceVersion {
		return Preview{}, ErrUnresolved
	}
	fresh, err := s.build(ctx, q, row, t, a)
	if err != nil {
		return Preview{}, contextError("ResolveApprovedInTransaction", err)
	}
	if fresh.Fingerprint != old.Fingerprint ||
		slices.Contains(
			fresh.Blockers,
			"unresolved_placeholders",
		) || slices.Contains(fresh.Blockers, "unresolved_official_fact") {
		return Preview{}, ErrUnresolved
	}
	fresh.ID = previewID
	fresh.Approved = true
	return fresh, nil
}

func (s Service) ResolveApproved(
	ctx context.Context,
	eventID, previewID uuid.UUID,
) (Preview, error) {
	var result Preview
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		result, err = s.ResolveApprovedInTransaction(ctx, tx, eventID, previewID)
		return contextError("ResolveApproved", err)
	})
	return result, contextError("ResolveApproved", err)
}

// RecordOfficialFact is called only by an owning domain inside its authorized
// source transaction. It is not an HTTP composer command. Callers must validate
// the source publication/status/ownership before recording this routing fact.
// It stores no private text; later readers additionally apply current grants.
func RecordOfficialFact(ctx context.Context, tx pgx.Tx, f dbgen.SaveCommunicationFactParams) error {
	if f.EventID == uuid.Nil || f.ID == uuid.Nil || f.Revision < 1 ||
		!slices.Contains(
			[]string{"help", "clarification", "rubric", "outcome_review", "results"},
			f.Kind,
		) ||
		!slices.Contains([]string{"open", "resolved", "published", "withdrawn"}, f.Status) {
		return access.ErrInvalid
	}
	if (f.Kind == "help" || f.Kind == "outcome_review") && f.OwnerID == nil {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	if _, err := q.LockEvent(ctx, f.EventID); err != nil {
		return contextError("RecordOfficialFact", err)
	}
	if _, err := q.SaveCommunicationFact(ctx, f); err != nil {
		return contextError("RecordOfficialFact", err)
	}
	_, err := q.BumpEventRevision(ctx, f.EventID)
	return contextError("RecordOfficialFact", err)
}
