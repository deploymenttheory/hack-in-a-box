package communications

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// PublicationPlanPreview displays literal template syntax and explicitly names
// deferred variables. It is not an attendee projection or generic approval.
type PublicationPlanPreview struct {
	PlanID                  uuid.UUID `json:"plan_id"`
	SourceID                uuid.UUID `json:"source_id"`
	SourceVersion           int64     `json:"source_version"`
	SubjectTemplate         string    `json:"subject_template"`
	BodyTemplate            string    `json:"body_template"`
	DeferredVariables       []string  `json:"deferred_variables"`
	EligibleRecipients      int       `json:"eligible_recipients"`
	GenericCampaignApproved bool      `json:"generic_campaign_approved"`
}

type publicationProof struct {
	ID                   uuid.UUID         `json:"id"`
	ActorID              uuid.UUID         `json:"actor_id"`
	CampaignID           uuid.UUID         `json:"campaign_id"`
	CampaignVersion      int64             `json:"campaign_version"`
	CampaignDigest       string            `json:"campaign_digest"`
	TemplateDigest       string            `json:"template_digest"`
	AudienceDigest       string            `json:"audience_digest"`
	ContentDigest        string            `json:"content_digest"`
	ConfigurationDigest  string            `json:"configuration_digest"`
	SourceDocumentDigest string            `json:"source_document_digest"`
	Source               publicationSource `json:"source"`
	ProviderRevisions    map[string]string `json:"provider_revisions"`
	DueAt                time.Time         `json:"due_at"`
	PreviewID            uuid.UUID         `json:"preview_id"`
	TemplateVersion      int64             `json:"template_version"`
	AudienceVersion      int64             `json:"audience_version"`
	Provenance           string            `json:"provenance"`
	ApprovedAt           time.Time         `json:"approved_at"`
}

func (s Service) publicationProof(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor, campaignID, sourceID uuid.UUID,
	version, sourceVersion int64,
) (publicationProof, Preview, Template, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return publicationProof{}, Preview{}, Template{}, contextError("lock publication plan", err)
	}
	if err = authorize(ctx, q, actor, eventID, true); err != nil {
		return publicationProof{}, Preview{}, Template{}, err
	}
	c, err := loadCampaign(ctx, q, eventID, campaignID)
	if err != nil {
		return publicationProof{}, Preview{}, Template{}, err
	}
	if c.Version != version || c.Cancelled {
		return publicationProof{}, Preview{}, Template{}, ErrUnresolved
	}
	basis, err := s.campaignBasis(ctx, q, row, actor)
	if err != nil {
		return publicationProof{}, Preview{}, Template{}, err
	}
	template, audience, err := basis.pair(c.Definition)
	if err != nil {
		return publicationProof{}, Preview{}, Template{}, err
	}
	source, err := q.ContentItem(ctx, dbgen.ContentItemParams{EventID: eventID, ID: sourceID})
	if err != nil {
		return publicationProof{}, Preview{}, Template{}, contextError(
			"read approved publication source",
			err,
		)
	}
	if err = validatePublicationTemplate(
		basis,
		source,
		sourceVersion,
		template,
		audience,
		c.Definition,
	); err != nil {
		return publicationProof{}, Preview{}, Template{}, err
	}
	due, err := campaignDue(basis.config, c.Definition.Timing)
	if err != nil || !due.After(s.now()) {
		return publicationProof{}, Preview{}, Template{}, ErrUnresolved
	}
	preview, err := s.build(ctx, q, row, template, audience)
	if err != nil {
		return publicationProof{}, Preview{}, Template{}, err
	}
	// The only deferred value is bound to this exact already-approved source.
	// Unknown or unavailable unrelated values continue to block plan review.
	for _, recipient := range preview.Recipients {
		for _, missing := range recipient.Unresolved {
			if missing != source.Kind+".title" {
				return publicationProof{}, Preview{}, Template{}, ErrUnresolved
			}
		}
	}
	if len(preview.Recipients) == 0 {
		return publicationProof{}, Preview{}, Template{}, ErrUnresolved
	}
	doc, err := q.ContentVersion(
		ctx,
		dbgen.ContentVersionParams{EventID: eventID, ContentID: sourceID, Version: sourceVersion},
	)
	if err != nil {
		return publicationProof{}, Preview{}, Template{}, contextError(
			"read immutable publication source",
			err,
		)
	}
	proof := publicationProof{
		ActorID:         actor,
		CampaignID:      campaignID,
		CampaignVersion: version,
		CampaignDigest:  digest(c.Definition),
		TemplateDigest:  digest(template),
		AudienceDigest:  digest(audience),
		ContentDigest:   digest(basis.content),
		ConfigurationDigest: digest(struct {
			Name   string
			Config any
		}{row.Name, basis.config}),
		SourceDocumentDigest: digest(json.RawMessage(doc.Document)),
		Source:               snapshotPublicationSource(source),
		ProviderRevisions:    map[string]string{},
		DueAt:                due,
		TemplateVersion:      template.Version,
		AudienceVersion:      audience.Version,
	}
	for _, channel := range c.Definition.Channels {
		for _, p := range basis.providers {
			if p.Channel == channel {
				proof.ProviderRevisions[channel] = p.ConfigRevision
			}
		}
	}
	proof.Provenance = digest(proof)
	return proof, preview, template, nil
}

func (s Service) PreviewExpectedPublicationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor, campaignID, sourceID uuid.UUID,
	version, sourceVersion int64,
	reason string,
) (PublicationPlanPreview, error) {
	proof, preview, template, err := s.publicationProof(
		ctx,
		tx,
		eventID,
		actor,
		campaignID,
		sourceID,
		version,
		sourceVersion,
	)
	if err != nil {
		return PublicationPlanPreview{}, err
	}
	proof.ID = uuid.New()
	proof.PreviewID = preview.ID
	q := dbgen.New(tx)
	raw, err := json.Marshal(preview)
	if err != nil {
		return PublicationPlanPreview{}, contextError("encode deferred plan preview", err)
	}
	if err = q.SaveCommunicationPreview(
		ctx,
		dbgen.SaveCommunicationPreviewParams{
			EventID:         eventID,
			ID:              preview.ID,
			TemplateID:      preview.TemplateID,
			TemplateVersion: preview.TemplateVersion,
			AudienceID:      preview.AudienceID,
			AudienceVersion: preview.AudienceVersion,
			ActorID:         actor,
			Snapshot:        raw,
		},
	); err != nil {
		return PublicationPlanPreview{}, contextError("save deferred plan preview", err)
	}
	if err = savePublicationProof(ctx, q, eventID, proof, false, nil, reason); err != nil {
		return PublicationPlanPreview{}, err
	}
	return PublicationPlanPreview{
		PlanID:             proof.ID,
		SourceID:           sourceID,
		SourceVersion:      sourceVersion,
		SubjectTemplate:    template.Subject,
		BodyTemplate:       template.Body,
		DeferredVariables:  []string{proof.Source.Kind + ".title"},
		EligibleRecipients: len(preview.Recipients),
	}, nil
}

func savePublicationProof(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	proof publicationProof,
	approved bool,
	parent *uuid.UUID,
	reason string,
) error {
	raw, err := json.Marshal(proof)
	if err != nil {
		return contextError("encode publication proof", err)
	}
	err = q.SavePublicationPlanApproval(
		ctx,
		dbgen.SavePublicationPlanApprovalParams{
			EventID:         eventID,
			ID:              proof.ID,
			CampaignID:      proof.CampaignID,
			CampaignVersion: proof.CampaignVersion,
			SourceID:        proof.Source.ID,
			SourceVersion:   proof.Source.Version,
			PreviewID:       proof.PreviewID,
			ActorID:         proof.ActorID,
			Document:        raw,
			Reason:          reason,
			Approved:        approved,
			PreviewPlanID:   parent,
		},
	)
	return contextError("save publication proof", err)
}

func (s Service) ApproveExpectedPublicationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor, previewID uuid.UUID,
	reason string,
) (ScopeApproval, error) {
	q := dbgen.New(tx)
	old, err := q.PublicationPlanApproval(
		ctx,
		dbgen.PublicationPlanApprovalParams{EventID: eventID, ID: previewID},
	)
	if err != nil {
		return ScopeApproval{}, contextError("read publication plan preview", err)
	}
	if old.Approved || old.ActorID != actor {
		return ScopeApproval{}, ErrUnresolved
	}
	var proof publicationProof
	if err = json.Unmarshal(old.Document, &proof); err != nil {
		return ScopeApproval{}, contextError("decode publication preview", err)
	}
	current, _, _, err := s.publicationProof(
		ctx,
		tx,
		eventID,
		actor,
		proof.CampaignID,
		proof.Source.ID,
		proof.CampaignVersion,
		proof.Source.Version,
	)
	if err != nil {
		return ScopeApproval{}, err
	}
	if current.Provenance != proof.Provenance {
		return ScopeApproval{}, ErrUnresolved
	}
	proof.ID = uuid.New()
	proof.ApprovedAt = s.now()
	if err = savePublicationProof(ctx, q, eventID, proof, true, &previewID, reason); err != nil {
		return ScopeApproval{}, err
	}
	return ScopeApproval{
		Kind:          "expected_publication",
		ID:            proof.ID,
		Binding:       proof.Provenance,
		SourceID:      proof.Source.ID,
		SourceVersion: proof.Source.Version,
	}, nil
}

func validatePublicationTemplate(
	basis campaignBasis,
	source dbgen.ContentItem,
	sourceVersion int64,
	template Template,
	audience Audience,
	definition CampaignDefinition,
) error {
	if source.Version != sourceVersion || source.State != "approved" || source.ApprovedBy == nil ||
		!source.ApprovedRevision.Valid ||
		source.ApprovedRevision.Int64 != source.Revision ||
		!basis.organisers[*source.ApprovedBy] ||
		!slices.Contains([]string{"challenge", "brief"}, source.Kind) {
		return ErrUnresolved
	}
	if template.Purpose != source.Kind+"_release" || template.FactID != nil ||
		audience.FactID != nil {
		return ErrUnresolved
	}
	if len(channelAllowed(definition.Channels, basis.providers)) > 0 {
		return ErrUnresolved
	}
	used, err := tokens(template.Subject + "\n" + template.Body)
	if err != nil {
		return err
	}
	for _, name := range used {
		if !slices.Contains(variables, name) ||
			(name == "brief.title" || name == "challenge.title") && name != source.Kind+".title" {
			return ErrUnresolved
		}
	}
	return nil
}

type publicationSource struct {
	EventID          uuid.UUID          `json:"event_id"`
	ID               uuid.UUID          `json:"id"`
	Kind             string             `json:"kind"`
	Revision         int64              `json:"revision"`
	Version          int64              `json:"version"`
	State            string             `json:"state"`
	ReleasedVersion  pgtype.Int8        `json:"released_version"`
	ScheduledAt      pgtype.Timestamptz `json:"scheduled_at"`
	ApprovedBy       *uuid.UUID         `json:"approved_by"`
	ApprovedRevision pgtype.Int8        `json:"approved_revision"`
	ReleasedAt       pgtype.Timestamptz `json:"released_at"`
}

func snapshotPublicationSource(item dbgen.ContentItem) publicationSource {
	return publicationSource{
		EventID:          item.EventID,
		ID:               item.ID,
		Kind:             item.Kind,
		Revision:         item.Revision,
		Version:          item.Version,
		State:            item.State,
		ReleasedVersion:  item.ReleasedVersion,
		ScheduledAt:      item.ScheduledAt,
		ApprovedBy:       item.ApprovedBy,
		ApprovedRevision: item.ApprovedRevision,
		ReleasedAt:       item.ReleasedAt,
	}
}

func (item publicationSource) contentItem() dbgen.ContentItem {
	return dbgen.ContentItem{
		EventID:          item.EventID,
		ID:               item.ID,
		Kind:             item.Kind,
		Revision:         item.Revision,
		Version:          item.Version,
		State:            item.State,
		ReleasedVersion:  item.ReleasedVersion,
		ScheduledAt:      item.ScheduledAt,
		ApprovedBy:       item.ApprovedBy,
		ApprovedRevision: item.ApprovedRevision,
		ReleasedAt:       item.ReleasedAt,
	}
}
