package submissions

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func nullableRevision(n int64) pgtype.Int8 { return pgtype.Int8{Int64: n, Valid: n > 0} }

func checkFiles(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	v scope,
	items []Evidence,
	now time.Time,
) error {
	for _, e := range items {
		if e.FileID == nil {
			continue
		}
		file, err := q.File(ctx, dbgen.FileParams{EventID: v.row.ID, ID: *e.FileID})
		if err != nil {
			return access.ErrForbidden
		}
		if file.TeamID == nil || *file.TeamID != v.entry.TeamID {
			return access.ErrForbidden
		}
		if err := (access.EventPolicy{Queries: q, Clock: func() time.Time { return now }}).Authorize(
			ctx,
			actor,
			v.row.ID,
			*e.FileID,
			access.FileRead,
		); err != nil {
			return fmt.Errorf("authorize submission attachment: %w", err)
		}
	}
	return nil
}

func deadlineChanged(defs []Definition, old, items []Evidence, now time.Time) bool {
	before, after := map[uuid.UUID]Evidence{}, map[uuid.UUID]Evidence{}
	for _, e := range old {
		before[e.DeliverableID] = e
	}
	for _, e := range items {
		after[e.DeliverableID] = e
	}
	for _, d := range defs {
		if !now.Before(d.DueAt) && !reflect.DeepEqual(before[d.ID], after[d.ID]) {
			return true
		}
	}
	return false
}

func saveState(ctx context.Context, q *dbgen.Queries, state dbgen.SubmissionState) error {
	if err := q.SaveSubmissionState(
		ctx,
		dbgen.SaveSubmissionStateParams(state),
	); err != nil {
		return fmt.Errorf("save submission outcome: %w", err)
	}
	return nil
}

func record(
	ctx context.Context,
	q *dbgen.Queries,
	state dbgen.SubmissionState,
	actor *uuid.UUID,
	action, status, reason string,
	now time.Time,
) error {
	if err := q.InsertSubmissionChange(
		ctx,
		dbgen.InsertSubmissionChangeParams{
			EventID:        state.EventID,
			TeamID:         state.TeamID,
			Revision:       state.Revision,
			ActorID:        actor,
			Action:         action,
			ActiveRevision: state.ActiveRevision,
			ReviewStatus:   status,
			Reason:         reason,
			OccurredAt:     now,
		},
	); err != nil {
		return fmt.Errorf("record submission history: %w", err)
	}
	code := audit.UserRequest
	if actor == nil {
		code = audit.DeadlinePolicy
	} else if reason != "" {
		code = audit.OrganiserDecision
	}
	auditAction := audit.SubmissionOutcomeChanged
	switch action {
	case "submitted":
		auditAction = audit.SubmissionFinalized
	case "reviewed":
		auditAction = audit.SubmissionReviewed
	}
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  actor,
			EventID:  &state.EventID,
			TargetID: state.TeamID,
			Action:   auditAction,
			Reason:   code,
			Metadata: audit.Metadata{Revision: &state.Revision},
		},
	); err != nil {
		return fmt.Errorf("audit submission change: %w", err)
	}
	return nil
}

func persistFiles(
	ctx context.Context,
	q *dbgen.Queries,
	state dbgen.SubmissionState,
	doc Document,
) error {
	defs := map[uuid.UUID]Definition{}
	refs := map[uuid.UUID][]string{}
	for _, d := range doc.Definitions {
		defs[d.ID] = d
	}
	for _, e := range doc.Evidence {
		if e.FileID != nil {
			for _, role := range audience(defs[e.DeliverableID]) {
				if !slices.Contains(refs[*e.FileID], role) {
					refs[*e.FileID] = append(refs[*e.FileID], role)
				}
			}
		}
	}
	for id, roles := range refs {
		if err := q.InsertSubmissionFileReference(
			ctx,
			dbgen.InsertSubmissionFileReferenceParams{
				EventID:            state.EventID,
				TeamID:             state.TeamID,
				SubmissionRevision: state.ActiveRevision.Int64,
				FileID:             id,
				Audience:           roles,
			},
		); err != nil {
			return fmt.Errorf("register formal attachment: %w", err)
		}
	}
	return nil
}

func (s Service) Submit(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	expected int64,
	cmd Command,
) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		now := s.now()
		if err := submissionWindow(v, expected, cmd, now); err != nil {
			return err
		}
		defs, err := definitions(ctx, q, v.row, v.config)
		if err != nil {
			return err
		}
		if err = validateEvidence(defs, cmd.Evidence, true); err != nil {
			return err
		}
		if err = checkFiles(ctx, q, actor, v, cmd.Evidence, now); err != nil {
			return err
		}
		old := []Evidence{}
		previousLate := false
		if v.state.ActiveRevision.Valid {
			r, err := revision(ctx, q, v, v.state.ActiveRevision.Int64)
			if err != nil {
				return err
			}
			old = r.Evidence
			previousLate = r.Late
		}
		late := deadlineChanged(defs, old, cmd.Evidence, now) || !open(v, now) ||
			v.state.FrozenAt.Valid
		if late && (!v.authority.Organiser || strings.TrimSpace(cmd.Reason) == "") {
			return blocked(
				"deliverables",
				"deadline_locked",
				"Changing evidence after its deadline requires an organiser reason.",
			)
		}
		doc := Document{Definitions: defs, Evidence: append([]Evidence{}, cmd.Evidence...)}
		data, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("encode formal submission: %w", err)
		}
		v.state.Revision++
		v.state.ActiveRevision = nullableRevision(v.state.Revision)
		v.state.Outcome = "open"
		if !open(v, now) || v.state.FrozenAt.Valid {
			v.state.Outcome = "frozen"
			if !v.state.FrozenAt.Valid {
				v.state.FrozenAt = pgtype.Timestamptz{Time: v.config.SubmissionCutoff, Valid: true}
			}
		}
		if err = q.InsertSubmissionRevision(
			ctx,
			dbgen.InsertSubmissionRevisionParams{
				EventID:     eventID,
				TeamID:      teamID,
				Revision:    v.state.Revision,
				ActorID:     actor,
				SubmittedAt: now,
				Late:        late || previousLate,
				Reason:      strings.TrimSpace(cmd.Reason),
				Document:    data,
			},
		); err != nil {
			return fmt.Errorf("append formal submission: %w", err)
		}
		if err = saveState(ctx, q, v.state); err != nil {
			return err
		}
		if err = persistFiles(ctx, q, v.state, doc); err != nil {
			return err
		}
		if err = record(
			ctx,
			q,
			v.state,
			&actor,
			"submitted",
			"pending",
			strings.TrimSpace(cmd.Reason),
			now,
		); err != nil {
			return err
		}
		if err = s.Settle(ctx, tx, v.row, now); err != nil {
			return err
		}
		result, err = s.page(ctx, q, v)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("submit competition entry: %w", err)
	}
	return result, nil
}

func (s Service) Review(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	expected, submissionRevision int64,
	status, reason string,
) (Page, error) {
	var result Page
	if !slices.Contains([]string{"pending", "accepted", "changes_requested"}, status) ||
		strings.TrimSpace(reason) == "" ||
		len(reason) > 2000 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		// This is organiser completion verification. Mentor checkpoint scores and final
		// judging use their own assigned assessment contracts in subsequent tickets.
		if !v.authority.Organiser || !v.authority.Active || !v.row.WritesAllowed {
			return access.ErrForbidden
		}
		if !live(v, s.now()) {
			return blocked(
				"live_start",
				"review_not_open",
				"Competition output cannot be reviewed before live start.",
			)
		}
		if v.state.Outcome == "withdrawn" || v.state.Outcome == "not_submitted" ||
			!v.state.ActiveRevision.Valid {
			return access.ErrForbidden
		}
		if v.state.Revision != expected || v.state.ActiveRevision.Int64 != submissionRevision {
			return access.ErrConflict
		}
		v.state.Revision++
		if err = saveState(ctx, q, v.state); err != nil {
			return err
		}
		if err = record(
			ctx,
			q,
			v.state,
			&actor,
			"reviewed",
			status,
			strings.TrimSpace(reason),
			s.now(),
		); err != nil {
			return err
		}
		result, err = s.page(ctx, q, v)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("review submitted evidence: %w", err)
	}
	return result, nil
}

func (s Service) Outcome(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	expected int64,
	outcome, reason string,
) (Page, error) {
	var result Page
	if !slices.Contains([]string{"withdrawn", "not_submitted"}, outcome) ||
		strings.TrimSpace(reason) == "" ||
		len(reason) > 2000 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		now := s.now()
		if !v.row.WritesAllowed || (!v.authority.Member && !v.authority.Organiser) {
			return access.ErrForbidden
		}
		if !published(v.row, now) {
			return access.ErrForbidden
		}
		if (!open(v, now) || v.state.FrozenAt.Valid || outcome == "not_submitted") &&
			!v.authority.Organiser {
			return access.ErrForbidden
		}
		if outcome == "not_submitted" && open(v, now) {
			return blocked(
				"submission_cutoff",
				"too_early",
				"Missing final submissions are recorded at the shared cutoff.",
			)
		}
		if v.state.Revision != expected {
			return access.ErrConflict
		}
		v.state.Revision++
		v.state.Outcome = outcome
		if !open(v, now) && !v.state.FrozenAt.Valid {
			v.state.FrozenAt = pgtype.Timestamptz{Time: v.config.SubmissionCutoff, Valid: true}
		}
		if outcome == "not_submitted" {
			v.state.ActiveRevision = pgtype.Int8{}
		}
		if err = saveState(ctx, q, v.state); err != nil {
			return err
		}
		if err = record(
			ctx,
			q,
			v.state,
			&actor,
			outcome,
			"",
			strings.TrimSpace(reason),
			now,
		); err != nil {
			return err
		}
		if err = s.Settle(ctx, tx, v.row, now); err != nil {
			return err
		}
		result, err = s.page(ctx, q, v)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("record submission outcome: %w", err)
	}
	return result, nil
}

func submissionWindow(v scope, expected int64, cmd Command, now time.Time) error {
	if !v.row.WritesAllowed || !v.authority.Active ||
		(!v.authority.Member && !v.authority.Organiser) {
		return access.ErrForbidden
	}
	if !live(v, now) {
		return blocked(
			"live_start",
			"not_open",
			"Formal submissions open at the scheduled live start.",
		)
	}
	exception := !open(v, now) || v.state.Outcome != "open" || v.state.FrozenAt.Valid
	if exception && (!v.authority.Organiser || strings.TrimSpace(cmd.Reason) == "") {
		return blocked(
			"submission_cutoff",
			"locked",
			"A locked or withdrawn entry requires a reasoned organiser amendment.",
		)
	}
	if v.state.Revision != expected {
		return access.ErrConflict
	}
	if len(cmd.Reason) > 2000 {
		return access.ErrInvalid
	}
	return nil
}
