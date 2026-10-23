package outcomes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func nativeApproval(proof ReviewProof) *communications.ScopeApproval {
	return &communications.ScopeApproval{
		Kind:          "native_outcome_review",
		ID:            proof.PlanID,
		Binding:       proof.Binding,
		SourceID:      proof.PlanID,
		SourceVersion: proof.PlanVersion,
	}
}

func (s Service) InspectOutcomePlanInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
	version int64,
) (communications.NativeOutcomePlanState, error) {
	q := dbgen.New(tx)
	event, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return communications.NativeOutcomePlanState{}, wrap("lock native review inspection", err)
	}
	if err = active(ctx, q, actor); err != nil {
		return communications.NativeOutcomePlanState{}, err
	}
	org, err := currentOrganiser(ctx, q, actor, &eventID)
	if err != nil {
		return communications.NativeOutcomePlanState{}, err
	}
	if !org {
		return communications.NativeOutcomePlanState{}, access.ErrForbidden
	}
	head, err := q.OutcomeReviewHead(ctx, id)
	if err != nil {
		return communications.NativeOutcomePlanState{}, wrap("inspect native plan", err)
	}
	if head.EventID != eventID || head.CurrentVersion != version {
		return communications.NativeOutcomePlanState{}, access.ErrConflict
	}
	row, err := q.OutcomeReviewVersion(
		ctx,
		dbgen.OutcomeReviewVersionParams{ReviewID: id, Version: version},
	)
	if err != nil {
		return communications.NativeOutcomePlanState{}, wrap("inspect exact plan revision", err)
	}
	v, err := reviewVersion(row)
	if err != nil {
		return communications.NativeOutcomePlanState{}, err
	}
	if v.ActorID != actor {
		return communications.NativeOutcomePlanState{}, access.ErrForbidden
	}
	out := communications.NativeOutcomePlanState{
		ID:      id,
		Version: version,
		State:   v.State,
		ActorID: actor,
		DueAt:   v.DueAt,
	}
	if v.State == "approved" {
		proof, e := s.resolveReview(ctx, tx, q, event, id, version, false)
		if e != nil {
			return out, e
		}
		out.Approval = nativeApproval(proof)
		out.RetainUntil = proof.RetainUntil
	}
	return out, nil
}

func (s Service) ResolveOutcomePlanInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
	version int64,
	binding string,
	personID uuid.UUID,
	literal, execution bool,
) (communications.ApprovedCampaign, error) {
	q := dbgen.New(tx)
	event, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return communications.ApprovedCampaign{}, wrap("lock native reminder resolution", err)
	}
	proof, err := s.resolveReview(ctx, tx, q, event, id, version, execution)
	if err != nil {
		return communications.ApprovedCampaign{}, err
	}
	if proof.Binding != binding || personID != uuid.Nil && personID != proof.OwnerID {
		return communications.ApprovedCampaign{}, access.ErrForbidden
	}
	person, err := q.Person(ctx, proof.OwnerID)
	if err != nil || !person.Active {
		return communications.ApprovedCampaign{}, access.ErrForbidden
	}
	loc, err := time.LoadLocation(proof.Schedule.Timezone)
	if err != nil {
		return communications.ApprovedCampaign{}, wrap("read approved review timezone", err)
	}
	values := map[string]string{
		"recipient.name": person.DisplayName,
		"outcome.url": strings.TrimRight(
			s.WorkspaceOrigin,
			"/",
		) + "/ideas/" + proof.OutcomeID.String(),
		"review.date": proof.DueAt.In(loc).
			Format("2006-01-02 15:04 MST") +
			" (" + proof.Schedule.Timezone + ")",
	}
	subject, a := communications.Render(proof.Template.Subject, "text", values)
	body, b := communications.Render(proof.Template.Body, "text", values)
	if len(a) > 0 || len(b) > 0 {
		return communications.ApprovedCampaign{}, communications.ErrUnresolved
	}
	channels := append([]string{}, proof.Schedule.Channels...)
	return communications.ApprovedCampaign{
		ScopeApproval: nativeApproval(proof),
		Purpose:       "outcome_review",
		Format:        "text",
		Definition: communications.CampaignDefinition{
			ID:         id,
			Name:       "Optional outcome review",
			TemplateID: id,
			AudienceID: id,
			Channels:   channels,
		},
		Version: version,
		DueAt:   proof.DueAt,
		Approval: communications.CampaignApproval{
			PreviewID:        id,
			TemplateVersion:  version,
			AudienceVersion:  version,
			Fingerprint:      proof.Binding,
			Binding:          proof.Binding,
			ActorID:          proof.ActorID,
			DueAt:            proof.DueAt,
			ChannelRevisions: proof.Channels,
		},
		Preview: communications.Preview{
			ID:              id,
			TemplateID:      id,
			TemplateVersion: version,
			AudienceID:      id,
			AudienceVersion: version,
			Fingerprint:     proof.Binding,
			Approved:        true,
			Recipients: []communications.Recipient{
				{
					PersonID:   proof.OwnerID,
					Name:       person.DisplayName,
					Subject:    subject,
					Body:       body,
					Unresolved: []string{},
				},
			},
			Blockers: []string{},
		},
	}, nil
}

// Historical reads retain the original approved recipient and native template;
// closing/rescheduling the review does not erase an already delivered message.
func (s Service) AuthorizeOutcomeInboxInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor uuid.UUID,
	p communications.InboxProvenance,
) error {
	q := dbgen.New(tx)
	if err := active(ctx, q, actor); err != nil {
		return err
	}
	a := p.ScopeApproval
	if a == nil || a.Kind != "native_outcome_review" || a.ID == uuid.Nil || a.ID != a.SourceID ||
		a.SourceVersion < 1 ||
		p.SelfOnly ||
		p.TemplateID != a.ID ||
		p.AudienceID != a.ID ||
		p.TemplateVersion != a.SourceVersion ||
		p.AudienceVersion != a.SourceVersion ||
		p.Resource == nil ||
		*p.Resource != (communications.ResourceReference{Kind: "outcome_review", ID: a.ID, Version: a.SourceVersion}) {
		return access.ErrNotFound
	}
	rh, err := q.OutcomeReviewHead(ctx, a.ID)
	if err != nil || rh.EventID != eventID {
		return access.ErrNotFound
	}
	row, err := q.OutcomeReviewVersion(
		ctx,
		dbgen.OutcomeReviewVersionParams{ReviewID: a.ID, Version: a.SourceVersion},
	)
	if err != nil {
		return access.ErrNotFound
	}
	v, err := reviewVersion(row)
	if err != nil {
		return err
	}
	var proof ReviewProof
	if json.Unmarshal(row.Proof, &proof) != nil || v.State != "approved" ||
		proof.Binding != a.Binding ||
		proof.OwnerID != actor ||
		proof.OutcomeID != rh.OutcomeID {
		return access.ErrNotFound
	}
	head, err := q.OutcomeHead(ctx, rh.OutcomeID)
	if err != nil {
		return wrap("read retained reminder resource", err)
	}
	if head.OwnerID == nil || *head.OwnerID != actor || head.CurrentVersion < proof.OutcomeVersion {
		return access.ErrNotFound
	}
	_, err = capabilities(ctx, q, actor, head, s.now())
	return err
}

func (s Service) installReviewComposition(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	v ReviewPlan,
) error {
	template := communications.Template{
		ID:      v.ID,
		Version: v.Version,
		Name:    "Native outcome review",
		Purpose: "outcome_review",
		Format:  "text",
		Subject: v.Message.Subject,
		Body:    v.Message.Body,
		FactID:  &v.ID,
	}
	audience := communications.Audience{
		ID:                 v.ID,
		Version:            v.Version,
		Name:               "Exact native current outcome owner",
		Outstanding:        "outcome_review",
		FactID:             &v.ID,
		Roles:              []string{},
		Teams:              []uuid.UUID{},
		RegistrationStates: []string{},
	}
	for kind, document := range map[string]any{"template": template, "audience": audience} {
		raw, err := json.Marshal(document)
		if err != nil {
			return wrap("encode retained review composition", err)
		}
		if err = q.SaveCommunicationVersion(
			ctx,
			dbgen.SaveCommunicationVersionParams{
				EventID:  eventID,
				ID:       v.ID,
				Kind:     kind,
				Version:  v.Version,
				Document: raw,
				ActorID:  v.ActorID,
			},
		); err != nil {
			return wrap("retain native review composition", err)
		}
	}
	campaign := communications.Campaign{
		Definition: communications.CampaignDefinition{
			ID:         v.ID,
			Name:       "Native outcome review (separate approval)",
			TemplateID: v.ID,
			AudienceID: v.ID,
			Channels:   v.Schedule.Channels,
		},
		Version:   v.Version,
		Cancelled: v.State == "cancelled",
	}
	raw, err := json.Marshal(campaign)
	if err != nil {
		return wrap("encode native campaign identity", err)
	}
	if err = q.SaveCommunicationCampaign(
		ctx,
		dbgen.SaveCommunicationCampaignParams{
			EventID:  eventID,
			ID:       v.ID,
			Version:  v.Version,
			Document: raw,
			ActorID:  v.ActorID,
			Reason:   "Native outcome plan composition; generic campaign remains unapproved",
		},
	); err != nil {
		return wrap("retain native campaign identity", err)
	}
	status := "withdrawn"
	if v.State == "approved" {
		status = "open"
	}
	var ownerID *uuid.UUID
	if v.OwnerID != uuid.Nil {
		ownerID = &v.OwnerID
	}
	if _, err = q.SaveCommunicationFact(
		ctx,
		dbgen.SaveCommunicationFactParams{
			EventID:    eventID,
			ID:         v.ID,
			Kind:       "outcome_review",
			Revision:   v.Version,
			OwnerID:    ownerID,
			ResourceID: nil,
			Status:     status,
		},
	); err != nil {
		return wrap("retain authoritative review routing metadata", err)
	}
	return nil
}

// ReviewSources supplies metadata only; the native resolver is authoritative and
// rejects unknown/stale rows rather than trusting the shared fact's owner field.
type ReviewSources struct{ Service *Service }

func (p ReviewSources) DiscoverInTransaction(
	_ context.Context,
	_ pgx.Tx,
	_ uuid.UUID,
	_ automation.Definition,
) ([]automation.Source, error) {
	return nil, communications.ErrUnresolved
}

func (p ReviewSources) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source automation.Source,
) (automation.SourceAudience, error) {
	if p.Service == nil || source.Kind != "outcome.follow_up" {
		return automation.SourceAudience{}, communications.ErrUnresolved
	}
	q := dbgen.New(tx)
	event, err := q.Event(ctx, eventID)
	if err != nil {
		return automation.SourceAudience{}, wrap("read review source event", err)
	}
	proof, err := p.Service.resolveReview(ctx, tx, q, event, source.ID, source.Revision, false)
	if err != nil {
		if errors.Is(err, access.ErrConflict) || errors.Is(err, access.ErrForbidden) ||
			errors.Is(err, access.ErrNotFound) {
			return automation.SourceAudience{}, communications.ErrUnresolved
		}
		return automation.SourceAudience{}, err
	}
	return automation.SourceAudience{
		Resource: &communications.ResourceReference{
			Kind:    "outcome_review",
			ID:      source.ID,
			Version: source.Revision,
		},
		RecipientIDs:     []uuid.UUID{proof.OwnerID},
		RequireComplete:  true,
		Broadcast:        false,
		RetainedFollowUp: true,
	}, nil
}

// AuthorizeReviewNotification checks the actual retained native resource. It
// does not require a pending send or current campaign approval to read history.
func (s Service) AuthorizeReviewNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
	version int64,
) error {
	q := dbgen.New(tx)
	head, err := q.OutcomeReviewHead(ctx, id)
	if err != nil || head.EventID != eventID {
		return access.ErrNotFound
	}
	row, err := q.OutcomeReviewVersion(
		ctx,
		dbgen.OutcomeReviewVersionParams{ReviewID: id, Version: version},
	)
	if err != nil {
		return access.ErrNotFound
	}
	v, err := reviewVersion(row)
	if err != nil {
		return err
	}
	if v.State != "approved" {
		return access.ErrNotFound
	}
	outcome, err := q.OutcomeHead(ctx, head.OutcomeID)
	if err != nil {
		return wrap("read notification's native outcome", err)
	}
	if outcome.OwnerID == nil || *outcome.OwnerID != actor || v.OwnerID != actor {
		return access.ErrNotFound
	}
	_, err = capabilities(ctx, q, actor, outcome, s.now())
	return err
}

func (s Service) AuthorizeCommunicationFact(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	f dbgen.CommunicationFact,
) (bool, error) {
	if f.Kind != "outcome_review" || f.Status != "open" || f.OwnerID == nil ||
		*f.OwnerID != actor ||
		f.ResourceID != nil {
		return false, nil
	}
	rh, err := q.OutcomeReviewHead(ctx, f.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, wrap("read actual outcome fact owner", err)
	}
	if rh.EventID != f.EventID || rh.CurrentVersion != f.Revision {
		return false, nil
	}
	allowed, err := q.OutcomeReviewSendable(
		ctx,
		dbgen.OutcomeReviewSendableParams{
			EventID:  f.EventID,
			ReviewID: f.ID,
			Version:  f.Revision,
			AtTime:   s.now(),
		},
	)
	return allowed, wrap("recheck native outcome fact", err)
}
