package communications

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ResolveApprovedScopedCampaignInTransaction never treats a generic draft as
// generically approved. A typed expected-publication approval is accepted only
// through its persisted approved occurrence and exact immutable plan record.
func (s Service) ResolveApprovedScopedCampaignInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope DeliveryScope,
	policy ScopePolicy,
) (ApprovedCampaign, error) {
	return s.resolveApprovedScopedCampaign(
		ctx,
		tx,
		eventID,
		campaignID,
		version,
		scope,
		policy,
		scopedProjection{},
	)
}

// ResolveApprovedScopedPersonalCampaignInTransaction uses the same persisted
// scope proof while rendering literal text for only the requested current person.
// A zero person resolves the batch during transactional queue creation.
func (s Service) ResolveApprovedScopedPersonalCampaignInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope DeliveryScope,
	policy ScopePolicy,
	personID uuid.UUID,
) (ApprovedCampaign, error) {
	return s.resolveApprovedScopedCampaign(
		ctx,
		tx,
		eventID,
		campaignID,
		version,
		scope,
		policy,
		scopedProjection{personID: personID, literal: true},
	)
}

type scopedProjection struct {
	personID uuid.UUID
	literal  bool
}

func (s Service) resolveApprovedScopedCampaign(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope DeliveryScope,
	policy ScopePolicy,
	projection scopedProjection,
) (ApprovedCampaign, error) {
	if scope.OccurrenceID == uuid.Nil || policy == nil {
		return ApprovedCampaign{}, access.ErrForbidden
	}
	audience, err := policy.ResolveInTransaction(ctx, tx, eventID, campaignID, version, scope)
	if err != nil {
		return ApprovedCampaign{}, contextError("resolve automation scope", err)
	}
	if audience.Binding == "" || len(audience.RecipientIDs) == 0 {
		return ApprovedCampaign{}, ErrUnresolved
	}
	if projection.personID != uuid.Nil &&
		!slices.Contains(audience.RecipientIDs, projection.personID) {
		return ApprovedCampaign{}, access.ErrForbidden
	}
	var approved ApprovedCampaign
	switch {
	case audience.Approval != nil && audience.Approval.Kind == "native_session_booking":
		if s.SessionPlans == nil {
			return ApprovedCampaign{}, access.ErrForbidden
		}
		approved, err = s.SessionPlans.ResolveSessionNotificationInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			version,
			*audience.Approval,
			projection.personID,
		)
	case audience.Approval != nil && audience.Approval.Kind == "native_outcome_review":
		if s.OutcomePlans == nil {
			return ApprovedCampaign{}, access.ErrForbidden
		}
		proof := audience.Approval
		if proof.ID != campaignID || proof.SourceID != campaignID ||
			proof.SourceVersion != version {
			return ApprovedCampaign{}, access.ErrForbidden
		}
		approved, err = s.OutcomePlans.ResolveOutcomePlanInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			version,
			proof.Binding,
			projection.personID,
			projection.literal,
			true,
		)
	case audience.Approval != nil && audience.Approval.Kind == "expected_publication":
		approved, err = s.resolveExpectedPublication(
			ctx,
			tx,
			eventID,
			campaignID,
			version,
			scope,
			audience,
			projection,
		)
	case audience.AllowArchived:
		approved, err = s.resolveRetainedFollowUp(
			ctx,
			tx,
			eventID,
			campaignID,
			version,
			scope,
			audience,
			projection,
		)
	default:
		approved, err = s.resolveApprovedPersonalCampaign(
			ctx,
			tx,
			eventID,
			campaignID,
			projection.personID,
			projection.literal,
		)
	}
	if err != nil {
		return ApprovedCampaign{}, err
	}
	if approved.Version != version {
		return ApprovedCampaign{}, access.ErrConflict
	}
	approved.Preview.Recipients = slices.DeleteFunc(
		approved.Preview.Recipients,
		func(r Recipient) bool { return !slices.Contains(audience.RecipientIDs, r.PersonID) },
	)
	if audience.RequireComplete {
		for _, id := range audience.RecipientIDs {
			// Queue creation validates the complete common projection; a worker
			// rechecks its one current recipient without rendering other people.
			if projection.personID != uuid.Nil && id != projection.personID {
				continue
			}
			if !slices.ContainsFunc(
				approved.Preview.Recipients,
				func(r Recipient) bool { return r.PersonID == id },
			) {
				return ApprovedCampaign{}, ErrUnresolved
			}
		}
	}
	if len(approved.Preview.Recipients) == 0 &&
		(audience.RequireComplete || projection.personID != uuid.Nil) {
		return ApprovedCampaign{}, ErrUnresolved
	}
	return approved, nil
}

func (s Service) resolveExpectedPublication(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope DeliveryScope,
	audience ScopeAudience,
	projection scopedProjection,
) (ApprovedCampaign, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return ApprovedCampaign{}, contextError("lock scoped publication", err)
	}
	proof, err := loadPublicationProof(ctx, q, eventID, campaignID, version, scope, audience)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	if err = authorize(ctx, q, proof.ActorID, eventID, true); err != nil {
		return ApprovedCampaign{}, err
	}
	c, err := loadCampaign(ctx, q, eventID, campaignID)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	if c.Version != version || c.Cancelled || digest(c.Definition) != proof.CampaignDigest {
		return ApprovedCampaign{}, ErrUnresolved
	}
	basis, err := s.campaignBasis(ctx, q, row, proof.ActorID)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	template, selector, err := basis.pair(c.Definition)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	if digest(template) != proof.TemplateDigest || digest(selector) != proof.AudienceDigest ||
		digest(struct {
			Name   string
			Config any
		}{row.Name, basis.config}) != proof.ConfigurationDigest {
		return ApprovedCampaign{}, ErrUnresolved
	}
	due, err := campaignDue(basis.config, c.Definition.Timing)
	if err != nil || !due.Equal(proof.DueAt) || s.now().Before(due) {
		return ApprovedCampaign{}, ErrUnresolved
	}
	if proof.Source.ApprovedBy == nil || !basis.organisers[*proof.Source.ApprovedBy] ||
		len(channelAllowed(c.Definition.Channels, basis.providers)) > 0 {
		return ApprovedCampaign{}, ErrUnresolved
	}
	for _, channel := range c.Definition.Channels {
		index := slices.IndexFunc(
			basis.providers,
			func(p ChannelReadiness) bool { return p.Channel == channel },
		)
		if index < 0 || proof.ProviderRevisions[channel] == "" ||
			basis.providers[index].ConfigRevision != proof.ProviderRevisions[channel] {
			return ApprovedCampaign{}, ErrUnresolved
		}
	}
	if err = s.verifyExpectedTransition(ctx, tx, q, eventID, row, basis, proof); err != nil {
		return ApprovedCampaign{}, err
	}
	// Only now invoke the ordinary current-reader renderer. No privileged source
	// text has been projected into a recipient before actual common publication.
	preview, err := s.buildProjection(
		ctx,
		q,
		row,
		template,
		selector,
		projection.personID,
		projection.literal,
	)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	preview.Recipients = slices.DeleteFunc(
		preview.Recipients,
		func(r Recipient) bool { return !slices.Contains(audience.RecipientIDs, r.PersonID) },
	)
	for _, r := range preview.Recipients {
		if len(r.Unresolved) > 0 {
			return ApprovedCampaign{}, ErrUnresolved
		}
	}
	if len(preview.Recipients) == 0 {
		return ApprovedCampaign{}, ErrUnresolved
	}
	for _, blocker := range preview.Blockers {
		if blocker != "unresolved_placeholders" {
			return ApprovedCampaign{}, ErrUnresolved
		}
	}
	preview.ID = proof.PreviewID
	preview.Approved = true
	preview.Blockers = []string{}
	approval := CampaignApproval{
		PreviewID:        proof.PreviewID,
		TemplateVersion:  proof.TemplateVersion,
		AudienceVersion:  proof.AudienceVersion,
		Fingerprint:      proof.Provenance,
		Binding:          proof.Provenance,
		ActorID:          proof.ActorID,
		ApprovedAt:       proof.ApprovedAt,
		DueAt:            proof.DueAt,
		ChannelRevisions: proof.ProviderRevisions,
	}
	return ApprovedCampaign{
		ScopeApproval: audience.Approval,
		Approval:      approval,
		Purpose:       template.Purpose,
		Format:        template.Format,
		Definition:    c.Definition,
		Version:       c.Version,
		DueAt:         proof.DueAt,
		Preview:       preview,
	}, nil
}

// Retained follow-up uses a separate narrow path; it cannot authorise ordinary
// operational messages or reopen event writes. Implemented alongside #82's
// retention/source guard, never as a caller-controlled skip-approval option.
func (s Service) resolveRetainedFollowUp(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope DeliveryScope,
	audience ScopeAudience,
	projection scopedProjection,
) (ApprovedCampaign, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return ApprovedCampaign{}, contextError("lock retained follow-up", err)
	}
	allowed, err := q.RetainedCommunicationOccurrence(
		ctx,
		dbgen.RetainedCommunicationOccurrenceParams{
			EventID:      eventID,
			OccurrenceID: scope.OccurrenceID,
			AtTime:       s.now(),
		},
	)
	if err != nil {
		return ApprovedCampaign{}, contextError("verify retained follow-up policy", err)
	}
	if !allowed || !audience.AllowArchived || audience.Approval == nil ||
		audience.Approval.Kind != "retained_followup" ||
		!slices.Contains([]string{"completed", "archived"}, row.Stage) {
		return ApprovedCampaign{}, ErrUnresolved
	}
	occurrence, err := q.AutomationOccurrence(
		ctx,
		dbgen.AutomationOccurrenceParams{EventID: eventID, ID: scope.OccurrenceID},
	)
	if err != nil {
		return ApprovedCampaign{}, contextError("read retained source reference", err)
	}
	if occurrence.ScopeBinding != audience.Binding ||
		occurrence.SourceKind != "outcome.follow_up" ||
		occurrence.SourceID != audience.Approval.SourceID ||
		occurrence.SourceRevision != audience.Approval.SourceVersion {
		return ApprovedCampaign{}, ErrUnresolved
	}
	c, err := loadCampaign(ctx, q, eventID, campaignID)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	if c.Cancelled || c.Version != version || c.Approval == nil ||
		c.Approval.Binding != audience.Approval.Binding {
		return ApprovedCampaign{}, ErrUnresolved
	}
	if err = authorize(ctx, q, c.Approval.ActorID, eventID, false); err != nil {
		return ApprovedCampaign{}, err
	}
	basis, err := s.campaignBasis(ctx, q, row, c.Approval.ActorID)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	view := campaignView(c, basis)
	if view.Status != "approved" || view.DueAt == nil || s.now().Before(*view.DueAt) {
		return ApprovedCampaign{}, ErrUnresolved
	}
	template, selector, err := basis.pair(c.Definition)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	if template.Purpose != "outcome_review" || template.FactID == nil ||
		*template.FactID != occurrence.SourceID ||
		slices.Contains(c.Definition.Channels, "teams") {
		return ApprovedCampaign{}, ErrUnresolved
	}
	fresh, err := s.retainedProjection(ctx, tx, row, c, template, selector, projection)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	return ApprovedCampaign{
		ScopeApproval: audience.Approval,
		Approval:      *c.Approval,
		Purpose:       template.Purpose,
		Format:        template.Format,
		Definition:    c.Definition,
		Version:       c.Version,
		DueAt:         *view.DueAt,
		Preview:       fresh,
	}, nil
}

func loadPublicationProof(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, campaignID uuid.UUID,
	version int64,
	scope DeliveryScope,
	audience ScopeAudience,
) (publicationProof, error) {
	occurrence, err := q.AutomationOccurrence(
		ctx,
		dbgen.AutomationOccurrenceParams{EventID: eventID, ID: scope.OccurrenceID},
	)
	if err != nil {
		return publicationProof{}, contextError("read publication occurrence", err)
	}
	binding, err := q.AutomationBinding(
		ctx,
		dbgen.AutomationBindingParams{EventID: eventID, ID: occurrence.BindingID},
	)
	if err != nil {
		return publicationProof{}, contextError("read publication binding", err)
	}
	var reference struct {
		PublicationApprovalID *uuid.UUID `json:"publication_approval_id"`
		Cancelled             bool       `json:"cancelled"`
	}
	if err = json.Unmarshal(binding.Document, &reference); err != nil {
		return publicationProof{}, contextError("decode publication reference", err)
	}
	if reference.Cancelled || reference.PublicationApprovalID == nil ||
		*reference.PublicationApprovalID != audience.Approval.ID ||
		binding.Version != occurrence.BindingVersion ||
		binding.CampaignID != campaignID ||
		binding.CampaignVersion != version ||
		occurrence.ScopeBinding != audience.Binding {
		return publicationProof{}, ErrUnresolved
	}
	record, err := q.PublicationPlanApproval(
		ctx,
		dbgen.PublicationPlanApprovalParams{EventID: eventID, ID: audience.Approval.ID},
	)
	if err != nil {
		return publicationProof{}, contextError("read typed publication approval", err)
	}
	if !record.Approved || record.CampaignID != campaignID || record.CampaignVersion != version {
		return publicationProof{}, ErrUnresolved
	}
	var proof publicationProof
	if err = json.Unmarshal(record.Document, &proof); err != nil {
		return publicationProof{}, contextError("decode typed publication approval", err)
	}
	if proof.Provenance != audience.Approval.Binding ||
		proof.Source.ID != audience.Approval.SourceID ||
		proof.Source.Version != audience.Approval.SourceVersion ||
		proof.ID != record.ID {
		return publicationProof{}, ErrUnresolved
	}
	return proof, nil
}

func (s Service) verifyExpectedTransition(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	eventID uuid.UUID,
	row dbgen.Event,
	basis campaignBasis,
	proof publicationProof,
) error {
	index := slices.IndexFunc(
		basis.content,
		func(item dbgen.ContentItem) bool { return item.ID == proof.Source.ID },
	)
	if index < 0 {
		return ErrUnresolved
	}
	source := basis.content[index]
	if source.State != "released" || source.Version != proof.Source.Version ||
		!source.ReleasedVersion.Valid ||
		source.ReleasedVersion.Int64 != proof.Source.Version ||
		source.Revision != proof.Source.Revision+1 ||
		!source.ReleasedAt.Valid ||
		s.now().Before(source.ReleasedAt.Time) ||
		!row.PublishedAt.Valid ||
		s.now().Before(row.PublishedAt.Time) {
		return ErrUnresolved
	}
	var recorded bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content_changes WHERE event_id=$1 AND content_id=$2 AND revision=$3 AND version=$4 AND action='released' AND occurred_at=$5)`, eventID, source.ID, source.Revision, source.Version, source.ReleasedAt.Time).
		Scan(&recorded); err != nil {
		return contextError("verify successful publication record", err)
	}
	if !recorded {
		return ErrUnresolved
	}
	normalized := source
	normalized.State = proof.Source.State
	normalized.Revision = proof.Source.Revision
	normalized.ReleasedVersion = proof.Source.ReleasedVersion
	normalized.ReleasedAt = proof.Source.ReleasedAt
	if digest(snapshotPublicationSource(normalized)) != digest(proof.Source) {
		return ErrUnresolved
	}
	basis.content[index] = proof.Source.contentItem()
	if digest(basis.content) != proof.ContentDigest {
		return ErrUnresolved
	}
	doc, err := q.ContentVersion(
		ctx,
		dbgen.ContentVersionParams{EventID: eventID, ContentID: source.ID, Version: source.Version},
	)
	if err != nil {
		return contextError("verify publication document", err)
	}
	if digest(json.RawMessage(doc.Document)) != proof.SourceDocumentDigest {
		return ErrUnresolved
	}
	return nil
}

func (s Service) retainedProjection(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	c Campaign,
	template Template,
	selector Audience,
	projection scopedProjection,
) (Preview, error) {
	q := dbgen.New(tx)
	eventID := row.ID
	approval, err := q.CommunicationApproval(
		ctx,
		dbgen.CommunicationApprovalParams{EventID: eventID, PreviewID: c.Approval.PreviewID},
	)
	if err != nil {
		return Preview{}, contextError("read retained preview approval", err)
	}
	if err = authorize(ctx, q, approval.ActorID, eventID, false); err != nil {
		return Preview{}, err
	}
	saved, err := q.CommunicationPreview(
		ctx,
		dbgen.CommunicationPreviewParams{EventID: eventID, ID: c.Approval.PreviewID},
	)
	if err != nil {
		return Preview{}, contextError("read retained preview", err)
	}
	var old Preview
	if err = json.Unmarshal(saved.Snapshot, &old); err != nil {
		return Preview{}, contextError("decode retained preview", err)
	}
	if saved.TemplateVersion != template.Version || saved.AudienceVersion != selector.Version {
		return Preview{}, ErrUnresolved
	}
	fresh, err := s.buildProjection(
		ctx,
		q,
		row,
		template,
		selector,
		projection.personID,
		projection.literal,
	)
	if err != nil {
		return Preview{}, err
	}
	if fresh.Fingerprint != old.Fingerprint || len(fresh.Blockers) > 0 {
		return Preview{}, ErrUnresolved
	}
	fresh.ID = c.Approval.PreviewID
	fresh.Approved = true
	return fresh, nil
}
