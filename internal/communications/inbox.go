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

// InboxProvenance retains exact immutable composition versions at delivery.
// It authorizes historical reads independently of later approval/cancellation,
// due-time or event write state. Dispatch still uses its stricter resolver.
type InboxProvenance struct {
	TemplateID      uuid.UUID `json:"template_id"`
	TemplateVersion int64     `json:"template_version"`
	AudienceID      uuid.UUID `json:"audience_id"`
	AudienceVersion int64     `json:"audience_version"`
	SelfOnly        bool      `json:"self_only"`
}

func (s Service) RetainInboxProvenanceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	selfOnly bool,
) (InboxProvenance, error) {
	q := dbgen.New(tx)
	if _, err := q.LockEvent(ctx, eventID); err != nil {
		return InboxProvenance{}, contextError("retain inbox event", err)
	}
	campaign, err := loadCampaign(ctx, q, eventID, campaignID)
	if err != nil {
		return InboxProvenance{}, err
	}
	template, audience, err := loadPair(
		ctx,
		q,
		eventID,
		campaign.Definition.TemplateID,
		campaign.Definition.AudienceID,
	)
	return InboxProvenance{
		TemplateID:      template.ID,
		TemplateVersion: template.Version,
		AudienceID:      audience.ID,
		AudienceVersion: audience.Version,
		SelfOnly:        selfOnly,
	}, contextError(
		"retain inbox sources",
		err,
	)
}

// AuthorizeInboxRecipientInTransaction only checks current read authority over
// the exact retained source versions. It never republishes or authorizes sends.
func (s Service) AuthorizeInboxRecipientInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor uuid.UUID,
	provenance InboxProvenance,
) error {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return contextError("inbox source event", err)
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return contextError("authorize inbox read", err)
	}
	if provenance.TemplateVersion < 1 || provenance.AudienceVersion < 1 {
		return access.ErrNotFound
	}
	templateRow, err := q.InboxCommunicationVersion(
		ctx,
		dbgen.InboxCommunicationVersionParams{
			EventID: eventID,
			ID:      provenance.TemplateID,
			Kind:    "template",
			Version: provenance.TemplateVersion,
		},
	)
	if err != nil {
		return access.ErrNotFound
	}
	audienceRow, err := q.InboxCommunicationVersion(
		ctx,
		dbgen.InboxCommunicationVersionParams{
			EventID: eventID,
			ID:      provenance.AudienceID,
			Kind:    "audience",
			Version: provenance.AudienceVersion,
		},
	)
	if err != nil {
		return access.ErrNotFound
	}
	var template Template
	var audience Audience
	if json.Unmarshal(templateRow.Document, &template) != nil ||
		json.Unmarshal(audienceRow.Document, &audience) != nil {
		return access.ErrNotFound
	}
	template.Version = templateRow.Version
	audience.Version = audienceRow.Version
	if provenance.SelfOnly {
		roles, roleErr := q.ActiveRoles(
			ctx,
			dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor},
		)
		if roleErr != nil {
			return contextError("inbox self role", roleErr)
		}
		if !slices.Contains(roles, "organiser") {
			return access.ErrNotFound
		}
		audience.Roles = []string{"organiser"}
		audience.RegistrationStates = []string{}
		audience.Teams = []uuid.UUID{}
		audience.Outstanding = ""
	}
	preview, err := s.build(ctx, q, row, template, audience)
	if err != nil {
		return contextError("authorize inbox read", err)
	}
	for _, recipient := range preview.Recipients {
		if recipient.PersonID == actor && len(recipient.Unresolved) == 0 {
			return nil
		}
	}
	return access.ErrNotFound
}
