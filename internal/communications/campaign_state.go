package communications

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

type campaignBasis struct {
	event     dbgen.Event
	config    event.Configuration
	composer  Page
	facts     []dbgen.CommunicationFact
	content   []dbgen.ContentItem
	providers []ChannelReadiness
}

func validateCampaign(d CampaignDefinition) error {
	if d.ID == uuid.Nil || d.TemplateID == uuid.Nil || d.AudienceID == uuid.Nil ||
		strings.TrimSpace(d.Name) == "" ||
		len(d.Name) > 160 ||
		len(d.Channels) < 1 ||
		len(d.Channels) > 3 {
		return access.ErrInvalid
	}
	seen := map[string]bool{}
	for _, ch := range d.Channels {
		if !slices.Contains(campaignChannels, ch) || seen[ch] {
			return access.ErrInvalid
		}
		seen[ch] = true
	}
	if d.IncludeCalendar && !seen["smtp"] {
		return access.ErrInvalid
	}
	t := d.Timing
	if !slices.Contains([]string{"", "earlier", "later"}, t.DSTChoice) {
		return access.ErrInvalid
	}
	switch t.Kind {
	case "fixed":
		if t.WallTime == "" || t.Anchor != "" || t.Days != 0 || t.Minutes != 0 || t.BusinessDays {
			return access.ErrInvalid
		}
	case "relative":
		if t.WallTime != "" ||
			!slices.Contains([]string{"live_start", "submission_cutoff"}, t.Anchor) ||
			t.Days < -366 ||
			t.Days > 366 ||
			t.Minutes < -1440 ||
			t.Minutes > 1440 {
			return access.ErrInvalid
		}
	default:
		return access.ErrInvalid
	}
	return nil
}

func campaignDue(c event.Configuration, t CampaignTiming) (time.Time, error) {
	if t.Kind == "fixed" {
		return schedule.ResolveLocal(t.WallTime, c.Timezone, t.DSTChoice)
	}
	c.DSTChoice = t.DSTChoice
	return event.Plan(
		c,
		event.AgendaItem{
			ID:           uuid.New(),
			Title:        "Campaign timing",
			Audience:     []string{"organiser"},
			Anchor:       t.Anchor,
			Days:         t.Days,
			Minutes:      t.Minutes,
			BusinessDays: t.BusinessDays,
		},
	)
}

func loadCampaign(ctx context.Context, q *dbgen.Queries, eventID, id uuid.UUID) (Campaign, error) {
	raw, err := q.CommunicationCampaign(
		ctx,
		dbgen.CommunicationCampaignParams{EventID: eventID, ID: id},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, access.ErrNotFound
	}
	if err != nil {
		return Campaign{}, contextError("load campaign", err)
	}
	var c Campaign
	if err = json.Unmarshal(raw.Document, &c); err != nil {
		return c, contextError("decode campaign", err)
	}
	return c, nil
}

func (s Service) campaignBasis(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
) (campaignBasis, error) {
	b := campaignBasis{event: row, config: event.DefaultConfiguration()}
	if err := json.Unmarshal(row.Configuration, &b.config); err != nil {
		return b, contextError("decode campaign timezone", err)
	}
	var err error
	b.composer, err = loadPage(ctx, q, row)
	if err != nil {
		return b, err
	}
	b.facts, err = q.CommunicationFacts(ctx, row.ID)
	if err != nil {
		return b, contextError("campaign facts", err)
	}
	b.content, err = q.ContentItems(
		ctx,
		dbgen.ContentItemsParams{EventID: row.ID, Cursor: uuid.Nil, PageSize: 1000},
	)
	if err != nil {
		return b, contextError("campaign content", err)
	}
	b.providers = s.channelReadiness(ctx, row.ID, actor)
	return b, nil
}

func (b campaignBasis) pair(d CampaignDefinition) (Template, Audience, error) {
	ti := slices.IndexFunc(
		b.composer.Templates,
		func(t Template) bool { return t.ID == d.TemplateID },
	)
	ai := slices.IndexFunc(
		b.composer.Audiences,
		func(a Audience) bool { return a.ID == d.AudienceID },
	)
	if ti < 0 || ai < 0 {
		return Template{}, Audience{}, access.ErrNotFound
	}
	return b.composer.Templates[ti], b.composer.Audiences[ai], nil
}

func campaignBinding(d CampaignDefinition, due time.Time, zone string, a CampaignApproval) string {
	return digest(struct {
		Definition                       CampaignDefinition
		DueAt                            time.Time
		Timezone                         string
		PreviewID                        uuid.UUID
		TemplateVersion, AudienceVersion int64
		Fingerprint                      string
	}{d, due, zone, a.PreviewID, a.TemplateVersion, a.AudienceVersion, a.Fingerprint})
}

func campaignView(c Campaign, b campaignBasis) CampaignView {
	v := CampaignView{Campaign: c, Status: "draft", Blockers: []string{}}
	if c.Cancelled {
		v.Status = "cancelled"
		return v
	}
	due, err := campaignDue(b.config, c.Definition.Timing)
	if err != nil {
		v.Blockers = append(v.Blockers, "timing_invalid")
	} else {
		v.DueAt = &due
	}
	t, a, err := b.pair(c.Definition)
	if err != nil {
		v.Blockers = append(v.Blockers, "message_or_audience_unavailable")
		return v
	}
	if slices.Contains(c.Definition.Channels, "teams") {
		v.Blockers = append(v.Blockers, broadcastBlockers(t, a, b.facts)...)
	}
	v.Blockers = append(v.Blockers, channelAllowed(c.Definition.Channels, b.providers)...)
	if c.Approval != nil {
		approved := *c.Approval
		if approved.TemplateVersion != t.Version || approved.AudienceVersion != a.Version ||
			approved.Fingerprint != approvalFingerprint(b.event, t, b.config, b.facts, b.content) ||
			v.DueAt == nil ||
			approved.Binding != campaignBinding(c.Definition, due, b.config.Timezone, approved) {
			v.Blockers = append(v.Blockers, "approval_outdated")
		} else {
			v.Status = "approved"
			if len(v.Blockers) > 0 {
				v.Status = "blocked"
			}
		}
	}
	return v
}
