package communications

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func campaignEvent(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	write bool,
) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, contextError("lock campaign event", err)
	}
	return row, authorize(ctx, q, actor, eventID, write)
}

func (s Service) Campaigns(ctx context.Context, actor, eventID uuid.UUID) (CampaignPage, error) {
	var result CampaignPage
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := campaignEvent(ctx, q, actor, eventID, false)
		if err != nil {
			return err
		}
		basis, err := s.campaignBasis(ctx, q, row, actor)
		if err != nil {
			return err
		}
		result = CampaignPage{
			Revision:  row.Revision,
			Timezone:  basis.config.Timezone,
			CanManage: row.WritesAllowed,
			Campaigns: []CampaignView{},
			Providers: basis.providers,
		}
		rows, err := q.CommunicationCampaigns(ctx, eventID)
		if err != nil {
			return contextError("list campaigns", err)
		}
		if len(rows) > 1000 {
			return blocked("too_many_campaigns")
		}
		for _, raw := range rows {
			var c Campaign
			if err = json.Unmarshal(raw.Document, &c); err != nil {
				return contextError("decode campaign list", err)
			}
			result.Campaigns = append(result.Campaigns, campaignView(c, basis))
		}
		return nil
	})
	return result, contextError("campaign index", err)
}

func (s Service) campaignDetail(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor, id uuid.UUID,
) (CampaignDetail, error) {
	c, err := loadCampaign(ctx, q, row.ID, id)
	if err != nil {
		return CampaignDetail{}, err
	}
	basis, err := s.campaignBasis(ctx, q, row, actor)
	if err != nil {
		return CampaignDetail{}, err
	}
	result := CampaignDetail{
		Revision:  row.Revision,
		Timezone:  basis.config.Timezone,
		CanManage: row.WritesAllowed,
		Campaign:  campaignView(c, basis),
		Providers: basis.providers,
	}
	if c.PreviewID != nil {
		raw, readErr := q.CommunicationPreview(
			ctx,
			dbgen.CommunicationPreviewParams{EventID: row.ID, ID: *c.PreviewID},
		)
		if readErr != nil {
			return result, contextError("read campaign preview", readErr)
		}
		var preview Preview
		if err = json.Unmarshal(raw.Snapshot, &preview); err != nil {
			return result, contextError("decode campaign preview", err)
		}
		preview.Approved = c.Approval != nil && result.Campaign.Status == "approved"
		result.Preview = &preview
	}
	return result, nil
}

func (s Service) Campaign(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (CampaignDetail, error) {
	var result CampaignDetail
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := campaignEvent(ctx, q, actor, eventID, false)
		if err != nil {
			return err
		}
		result, err = s.campaignDetail(ctx, q, row, actor, id)
		return err
	})
	return result, contextError("read campaign", err)
}

func saveCampaign(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Campaign,
	reason string,
) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return contextError("encode campaign", err)
	}
	if err = q.SaveCommunicationCampaign(
		ctx,
		dbgen.SaveCommunicationCampaignParams{
			EventID:  row.ID,
			ID:       c.Definition.ID,
			Version:  c.Version,
			Document: raw,
			ActorID:  actor,
			Reason:   reason,
		},
	); err != nil {
		return contextError("save campaign", err)
	}
	return contextError(
		"cancel obsolete campaign tests",
		q.CancelCommunicationCampaignTests(
			ctx,
			dbgen.CancelCommunicationCampaignTestsParams{
				EventID:    row.ID,
				CampaignID: c.Definition.ID,
			},
		),
	)
}

func (s Service) ChangeCampaign(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	revision int64,
	input CampaignCommand,
) (CampaignDetail, error) {
	var result CampaignDetail
	if strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 || input.Version < 0 ||
		!slices.Contains(
			[]string{"save", "preview", "approve", "cancel", "test_self"},
			input.Action,
		) ||
		(input.Action == "save") != (input.Definition != nil) ||
		(input.Action == "test_self") != (input.Channel != "") ||
		(input.ReviewedTestFingerprint != "" && (input.Action != "test_self" || !validReviewFingerprint(input.ReviewedTestFingerprint))) {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := campaignEvent(ctx, q, actor, eventID, true)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		current, err := loadCampaign(ctx, q, eventID, id)
		if errors.Is(err, access.ErrNotFound) && input.Action == "save" && input.Version == 0 {
			current = Campaign{Definition: CampaignDefinition{ID: id}}
		} else if err != nil {
			return err
		}
		if current.Version != input.Version {
			return access.ErrConflict
		}
		var testID *uuid.UUID
		if input.Action == "test_self" {
			testID, err = s.queueTestSelf(ctx, tx, q, row, actor, current, input)
		} else {
			current, err = s.applyCampaign(ctx, q, row, actor, current, input)
			if err == nil {
				current.Version++
				err = saveCampaign(ctx, q, row, actor, current, input.Reason)
			}
		}
		if err != nil {
			return err
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: id,
				Action:   audit.CommunicationChanged,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{ReferenceID: testID},
			},
		); err != nil {
			return contextError("audit campaign change", err)
		}
		row, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return contextError("campaign revision", err)
		}
		result, err = s.campaignDetail(ctx, q, row, actor, id)
		result.TestRequestID = testID
		return err
	})
	return result, contextError("change campaign", err)
}

func (s Service) applyCampaign(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Campaign,
	input CampaignCommand,
) (Campaign, error) {
	switch input.Action {
	case "save":
		d := *input.Definition
		if d.ID != c.Definition.ID {
			return c, access.ErrInvalid
		}
		if err := validateCampaign(d); err != nil {
			return c, err
		}
		basis, err := s.campaignBasis(ctx, q, row, actor)
		if err != nil {
			return c, err
		}
		if _, _, err = basis.pair(d); err != nil {
			return c, err
		}
		if _, err = campaignDue(basis.config, d.Timing); err != nil {
			return c, &event.Blocked{Issues: []event.Issue{{
				Field:   "timing",
				Code:    "campaign_timing_invalid",
				Message: err.Error(),
			}}}
		}
		c.Definition = d
		c.Approval = nil
		c.PreviewID = nil
		c.Cancelled = false
		return c, nil
	case "cancel":
		c.Cancelled = true
		c.Approval = nil
		return c, nil
	case "preview":
		return s.previewCampaign(ctx, q, row, actor, c)
	case "approve":
		return s.approveCampaign(ctx, q, row, actor, c, input.Reason)
	default:
		return c, access.ErrInvalid
	}
}

func (s Service) previewCampaign(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Campaign,
) (Campaign, error) {
	if c.Cancelled {
		return c, blocked("campaign_cancelled")
	}
	t, a, err := loadPair(ctx, q, row.ID, c.Definition.TemplateID, c.Definition.AudienceID)
	if err != nil {
		return c, err
	}
	p, err := s.build(ctx, q, row, t, a)
	if err != nil {
		return c, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return c, contextError("encode campaign preview", err)
	}
	err = q.SaveCommunicationPreview(
		ctx,
		dbgen.SaveCommunicationPreviewParams{
			EventID:         row.ID,
			ID:              p.ID,
			TemplateID:      t.ID,
			TemplateVersion: t.Version,
			AudienceID:      a.ID,
			AudienceVersion: a.Version,
			ActorID:         actor,
			Snapshot:        raw,
		},
	)
	c.PreviewID = &p.ID
	c.Approval = nil
	return c, contextError("save campaign preview", err)
}

func (s Service) approveCampaign(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Campaign,
	reason string,
) (Campaign, error) {
	if c.Cancelled || c.PreviewID == nil {
		return c, blocked("campaign_preview_required")
	}
	basis, err := s.campaignBasis(ctx, q, row, actor)
	if err != nil {
		return c, err
	}
	view := campaignView(c, basis)
	if view.DueAt == nil || !view.DueAt.After(s.now()) {
		return c, blocked("campaign_timing_review_required")
	}
	for _, code := range view.Blockers {
		if code != "approval_outdated" && code != "provider_configuration_changed" {
			return c, blocked(code)
		}
	}
	if previewActor, exists := basis.previewApprovers[*c.PreviewID]; exists &&
		!basis.organisers[previewActor] {
		return c, blocked("preview_approver_authority_review_required")
	}
	preview, err := s.approve(ctx, q, row, actor, Command{PreviewID: *c.PreviewID, Reason: reason})
	if err != nil {
		return c, err
	}
	a := CampaignApproval{
		PreviewID:       preview.ID,
		TemplateVersion: preview.TemplateVersion,
		AudienceVersion: preview.AudienceVersion,
		Fingerprint:     preview.Fingerprint,
		ActorID:         actor,
		ApprovedAt:      s.now(),
		DueAt:           *view.DueAt,
	}
	a.ChannelRevisions = map[string]string{}
	for _, provider := range basis.providers {
		if slices.Contains(c.Definition.Channels, provider.Channel) {
			a.ChannelRevisions[provider.Channel] = provider.ConfigRevision
		}
	}
	a.Binding = campaignBinding(c.Definition, a.DueAt, basis.config.Timezone, a)
	c.Approval = &a
	return c, nil
}
