package outcomes

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func reviewVersion(row dbgen.OutcomeReviewVersion) (ReviewPlan, error) {
	var v ReviewPlan
	if err := json.Unmarshal(row.Document, &v); err != nil {
		return v, wrap("decode immutable review plan", err)
	}
	if v.ID != row.ReviewID || v.Version != row.Version || v.CommandID != row.CommandID ||
		v.ActorID != row.ActorID {
		return v, access.ErrConflict
	}
	return v, nil
}

func reviewCommandEqual(raw []byte, c PlanCommand) bool {
	var old PlanCommand
	if json.Unmarshal(raw, &old) != nil {
		return false
	}
	return proofDigest(old) == proofDigest(c)
}

func (s Service) ChangePlan(
	ctx context.Context,
	actor, outcomeID, reviewID uuid.UUID,
	expected int64,
	c PlanCommand,
) (PlanView, error) {
	var out PlanView
	if reviewID == uuid.Nil || expected < 0 {
		return out, access.ErrInvalid
	}
	if err := c.Validate(); err != nil {
		return out, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := lockWrite(ctx, q, outcomeID, nil)
		if err != nil {
			return err
		}
		event, err := requirePublicationSource(ctx, q, actor, head, s.now())
		if err != nil {
			return err
		}
		if c.Action == "save" && expected == 0 {
			if err = q.CreateOutcomeReviewHead(
				ctx,
				dbgen.CreateOutcomeReviewHeadParams{
					ID:        reviewID,
					EventID:   event.ID,
					OutcomeID: outcomeID,
					Days:      map[int]int32{30: 30, 60: 60, 90: 90}[c.Schedule.Days],
				},
			); err != nil {
				return wrap("create review identity", err)
			}
		}
		rh, err := q.OutcomeReviewHead(ctx, reviewID)
		if err != nil {
			return wrap("read review identity", err)
		}
		if rh.OutcomeID != outcomeID || rh.EventID != event.ID {
			return access.ErrNotFound
		}
		receipt, replayErr := q.OutcomeReviewCommand(
			ctx,
			dbgen.OutcomeReviewCommandParams{ReviewID: reviewID, CommandID: c.ID},
		)
		if replayErr == nil {
			if receipt.ActorID != actor || receipt.ExpectedVersion != expected ||
				!reviewCommandEqual(receipt.Command, c) {
				return access.ErrConflict
			}
			v, readErr := reviewVersion(receipt)
			if readErr != nil {
				return readErr
			}
			out, readErr = s.planView(ctx, tx, q, event, rh, v)
			return readErr
		}
		if !errors.Is(replayErr, pgx.ErrNoRows) {
			return wrap("read exact review command", replayErr)
		}
		if rh.CurrentVersion != expected || head.CurrentVersion != c.SourceVersion {
			return access.ErrConflict
		}
		source, err := load(ctx, q, outcomeID, head.CurrentVersion)
		if err != nil {
			return err
		}
		v := ReviewPlan{ID: reviewID, OutcomeID: outcomeID}
		if expected > 0 {
			prior, e := q.OutcomeReviewVersion(
				ctx,
				dbgen.OutcomeReviewVersionParams{ReviewID: reviewID, Version: expected},
			)
			if e != nil {
				return wrap("read review base", e)
			}
			v, e = reviewVersion(prior)
			if e != nil {
				return e
			}
		}
		v.Version = expected + 1
		v.CommandID = c.ID
		v.ActorID = actor
		v.At = s.now()
		v.Reason = c.Reason
		proof, err := s.applyPlan(ctx, q, event, head, source, &v, c)
		if err != nil {
			return err
		}
		if err = appendPlan(ctx, q, v, c, expected, proof); err != nil {
			return err
		}
		rh.CurrentVersion = v.Version
		rh.State = v.State
		out, err = s.planView(ctx, tx, q, event, rh, v)
		return err
	})
	return out, wrap("change review plan", err)
}

func (s Service) applyPlan(
	ctx context.Context,
	q *dbgen.Queries,
	event dbgen.Event,
	head dbgen.OutcomeHead,
	source Version,
	v *ReviewPlan,
	c PlanCommand,
) (ReviewProof, error) {
	var proof ReviewProof
	switch c.Action {
	case "save":
		if c.Schedule.Days != int(v.Schedule.Days) && v.Schedule.Days != 0 {
			return proof, access.ErrInvalid
		}
		due, err := schedule.ResolveLocal(
			c.Schedule.WallTime,
			c.Schedule.Timezone,
			c.Schedule.DSTChoice,
		)
		if err != nil {
			return proof, access.ErrInvalid
		}
		if !due.After(s.now()) {
			return proof, access.ErrInvalid
		}
		v.Schedule = *c.Schedule
		v.Message = *c.Message
		v.DueAt = due
		v.SourceVersion = source.Version
		v.State = "draft"
		if head.OwnerID != nil {
			v.OwnerID = *head.OwnerID
		} else {
			v.OwnerID = uuid.Nil
		}
	case "approve":
		if !v.DueAt.After(s.now()) || v.State != "draft" || v.SourceVersion != source.Version ||
			head.OwnerID == nil ||
			v.OwnerID != *head.OwnerID ||
			!slices.Contains([]string{"accepted", "in_progress"}, source.Status) {
			return proof, access.ErrConflict
		}
		var err error
		proof, err = s.approvalProof(ctx, q, event, head, *v)
		if err != nil {
			return proof, err
		}
		v.State = "approved"
	case "cancel":
		if v.Version == 1 {
			return proof, access.ErrConflict
		}
		v.State = "cancelled"
	default:
		return proof, access.ErrInvalid
	}
	return proof, nil
}

func (s Service) approvalProof(
	ctx context.Context,
	q *dbgen.Queries,
	eventRow dbgen.Event,
	head dbgen.OutcomeHead,
	v ReviewPlan,
) (ReviewProof, error) {
	proof := ReviewProof{
		ConfigurationBinding: proofDigest(json.RawMessage(eventRow.Configuration)),
		PlanID:               v.ID,
		PlanVersion:          v.Version,
		OutcomeID:            head.ID,
		OutcomeVersion:       head.CurrentVersion,
		OwnerID:              v.OwnerID,
		ActorID:              v.ActorID,
		Template:             v.Message,
		DueAt:                v.DueAt,
		Channels:             map[string]string{},
	}
	if eventRow.Stage == "cancelled" || s.Composer == nil {
		return proof, access.ErrConflict
	}
	eligible, err := q.OutcomeEligibleOwner(
		ctx,
		dbgen.OutcomeEligibleOwnerParams{OutcomeID: head.ID, PersonID: v.OwnerID},
	)
	if err != nil {
		return proof, wrap("check approved review owner", err)
	}
	if !eligible {
		return proof, access.ErrConflict
	}
	policy, err := q.PrivacyPolicy(ctx, eventRow.ID)
	if err != nil {
		return proof, wrap("read review retention", err)
	}
	// Before completion this is a conservative bound from the configured live end;
	// completion/rescheduling changes must be reviewed, never silently extended.
	anchor := eventRow.CompletedAt.Time
	if !eventRow.CompletedAt.Valid {
		var cfg event.Configuration
		if e := json.Unmarshal(eventRow.Configuration, &cfg); e != nil {
			return proof, wrap("read retention anchor", e)
		}
		anchor = cfg.LiveEnd
	}
	proof.RetainUntil = anchor.AddDate(0, 0, int(policy.RetentionDays))
	if !s.now().Before(proof.RetainUntil) || v.DueAt.After(proof.RetainUntil) {
		return proof, access.ErrInvalid
	}
	proof.PrivacyRevision, err = q.OutcomePrivacyRevision(ctx, eventRow.ID)
	if err != nil {
		return proof, wrap("read review policy revision", err)
	}
	for _, channel := range v.Schedule.Channels {
		provider := s.Composer.Channels[channel]
		if provider == nil {
			return proof, access.ErrConflict
		}
		ready, e := provider.Readiness(ctx, v.ActorID, eventRow.ID)
		if e != nil {
			return proof, wrap("read review provider", e)
		}
		if !ready.Available || ready.ConfigRevision == "" {
			return proof, access.ErrConflict
		}
		proof.Channels[channel] = ready.ConfigRevision
	}
	proof.Binding = proofDigest(proof)
	return proof, nil
}

func appendPlan(
	ctx context.Context,
	q *dbgen.Queries,
	v ReviewPlan,
	c PlanCommand,
	expected int64,
	proof ReviewProof,
) error {
	document, err := json.Marshal(v)
	if err != nil {
		return wrap("encode review plan", err)
	}
	command, err := json.Marshal(c)
	if err != nil {
		return wrap("encode review command", err)
	}
	raw, err := json.Marshal(proof)
	if err != nil {
		return wrap("encode native approval", err)
	}
	if err = q.AppendOutcomeReviewVersion(
		ctx,
		dbgen.AppendOutcomeReviewVersionParams{
			ReviewID:        v.ID,
			Version:         v.Version,
			CommandID:       c.ID,
			ActorID:         v.ActorID,
			ExpectedVersion: expected,
			Command:         command,
			Document:        document,
			Proof:           raw,
			CreatedAt:       v.At,
		},
	); err != nil {
		return wrap("append review version", err)
	}
	if err = q.UpdateOutcomeReviewHead(
		ctx,
		dbgen.UpdateOutcomeReviewHeadParams{ID: v.ID, CurrentVersion: v.Version, State: v.State},
	); err != nil {
		return wrap("advance review plan", err)
	}
	head, err := q.OutcomeHead(ctx, v.OutcomeID)
	if err != nil {
		return wrap("read review audit identity", err)
	}
	if err = audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &v.ActorID,
			EventID:  head.EventID,
			TargetID: v.ID,
			Action:   audit.OutcomeReviewChanged,
			Reason:   audit.OrganiserDecision,
			Metadata: audit.Metadata{Revision: &v.Version, PreviousRevision: &expected},
		},
	); err != nil {
		return wrap("audit explicit review decision", err)
	}
	return nil
}

func (s Service) planView(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	event dbgen.Event,
	head dbgen.OutcomeReviewHead,
	v ReviewPlan,
) (PlanView, error) {
	out := PlanView{
		ReviewPlan:     v,
		CurrentVersion: head.CurrentVersion,
		EffectiveState: v.State,
		Blockers:       []string{},
	}
	if v.State != "approved" || v.Version != head.CurrentVersion {
		return out, nil
	}
	if _, err := s.resolveReview(ctx, tx, q, event, v.ID, v.Version, false); err != nil {
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrConflict) ||
			errors.Is(err, access.ErrNotFound) {
			out.EffectiveState = "review_required"
			out.Blockers = []string{"current_approval_no_longer_eligible"}
			return out, nil
		}
		return out, err
	}
	return out, nil
}

// resolveReview is the authoritative native source check. Readability and
// execution eligibility remain distinct: historical inbox reads do not call it.
func (s Service) resolveReview(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	event dbgen.Event,
	id uuid.UUID,
	version int64,
	execution bool,
) (ReviewProof, error) {
	head, err := q.OutcomeReviewHead(ctx, id)
	if err != nil {
		return ReviewProof{}, wrap("read current review source", err)
	}
	if head.EventID != event.ID || head.CurrentVersion != version || head.State != "approved" {
		return ReviewProof{}, access.ErrConflict
	}
	row, err := q.OutcomeReviewVersion(
		ctx,
		dbgen.OutcomeReviewVersionParams{ReviewID: id, Version: version},
	)
	if err != nil {
		return ReviewProof{}, wrap("read native review approval", err)
	}
	v, err := reviewVersion(row)
	if err != nil {
		return ReviewProof{}, err
	}
	var proof ReviewProof
	if err = json.Unmarshal(row.Proof, &proof); err != nil {
		return proof, wrap("read exact approval proof", err)
	}
	if proof.PlanID != id || proof.PlanVersion != version || proof.ActorID != v.ActorID ||
		proof.Binding == "" {
		return proof, access.ErrConflict
	}
	if err = active(ctx, q, proof.ActorID); err != nil {
		return proof, err
	}
	authorised, err := currentOrganiser(ctx, q, proof.ActorID, &event.ID)
	if err != nil {
		return proof, err
	}
	if !authorised {
		return proof, access.ErrForbidden
	}
	outcome, err := q.OutcomeHead(ctx, head.OutcomeID)
	if err != nil {
		return proof, wrap("read current follow-through source", err)
	}
	if outcome.CurrentVersion != proof.OutcomeVersion || outcome.OwnerID == nil ||
		*outcome.OwnerID != proof.OwnerID ||
		!slices.Contains([]string{"accepted", "in_progress"}, outcome.Status) {
		return proof, access.ErrConflict
	}
	cap, err := capabilities(ctx, q, proof.OwnerID, outcome, s.now())
	if err != nil {
		return proof, err
	}
	if !cap.CanFollowUp {
		return proof, access.ErrForbidden
	}
	current, err := s.approvalProof(ctx, q, event, outcome, v)
	if err != nil {
		return proof, err
	}
	if current.Binding != proof.Binding || !s.now().Before(proof.RetainUntil) {
		return proof, access.ErrConflict
	}
	if execution && s.now().Before(proof.DueAt) {
		return proof, access.ErrConflict
	}
	return proof, nil
}

func (s Service) Plans(ctx context.Context, actor, outcomeID uuid.UUID) (PlanPage, error) {
	out := PlanPage{Items: []PlanView{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := q.OutcomeHead(ctx, outcomeID)
		if err != nil {
			return wrap("read plan resource", err)
		}
		if _, err = capabilities(ctx, q, actor, head, s.now()); err != nil {
			return err
		}
		if head.EventID == nil {
			return nil
		}
		event, err := q.Event(ctx, *head.EventID)
		if err != nil {
			return wrap("read plan event", err)
		}
		heads, err := q.OutcomeReviewHeads(ctx, outcomeID)
		if err != nil {
			return wrap("list optional review plans", err)
		}
		for _, rh := range heads {
			row, e := q.OutcomeReviewVersion(
				ctx,
				dbgen.OutcomeReviewVersionParams{ReviewID: rh.ID, Version: rh.CurrentVersion},
			)
			if e != nil {
				return wrap("read listed review version", e)
			}
			v, e := reviewVersion(row)
			if e != nil {
				return e
			}
			view, e := s.planView(ctx, tx, q, event, rh, v)
			if e != nil {
				return e
			}
			out.Items = append(out.Items, view)
		}
		return nil
	})
	return out, wrap("review plans", err)
}
