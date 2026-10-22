package automation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func hash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func sourceKey(source Source) string {
	return fmt.Sprintf("%s:%s:%d", source.Kind, source.ID, source.Revision)
}

func scopeBinding(binding Binding, source Source, review int64) string {
	return hash(struct {
		Binding   Binding
		SourceKey string
		Review    int64
	}{binding, sourceKey(source), review})
}

func currentCampaign(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
) (communications.Campaign, error) {
	row, err := q.CommunicationCampaign(
		ctx,
		dbgen.CommunicationCampaignParams{EventID: eventID, ID: id},
	)
	if err != nil {
		return communications.Campaign{}, fmt.Errorf("read automation campaign: %w", err)
	}
	var campaign communications.Campaign
	if err = json.Unmarshal(row.Document, &campaign); err != nil {
		return campaign, fmt.Errorf("decode automation campaign: %w", err)
	}
	return campaign, nil
}

func (s Service) sourcePolicy(kind string) SourcePolicy {
	if p := s.Sources[kind]; p != nil {
		return p
	}
	return builtinSource{service: s, kind: kind}
}

// sourceAvailable excludes declared future journeys until their owning domain
// has supplied an authoritative adapter. Saving their draft is still permitted.
func (s Service) sourceAvailable(kind string) bool {
	if s.Sources[kind] != nil {
		return true
	}
	return slices.Contains(
		[]string{
			"scheduled",
			"preparation.reminder",
			"follow_up",
			"outcome.follow_up",
			"registration.offer",
			"registration.place_changed",
			"facilitator.selected",
			"teams.published",
			"mentors.published",
			"event.rescheduled",
			"challenge.released",
			"brief.released",
		},
		kind,
	)
}

func commonRecipients(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	now time.Time,
) ([]uuid.UUID, error) {
	people, err := q.CommunicationPeople(
		ctx,
		dbgen.CommunicationPeopleParams{EventID: eventID, At: now},
	)
	if err != nil {
		return nil, fmt.Errorf("resolve current automation people: %w", err)
	}
	if len(people) > 10000 {
		return nil, access.ErrInvalid
	}
	ids := make([]uuid.UUID, 0, len(people))
	for _, person := range people {
		ids = append(ids, person.ID)
	}
	return ids, nil
}

func (s Service) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
) (communications.ScopeAudience, error) {
	return s.resolveScope(ctx, tx, eventID, campaignID, version, scope, true)
}

// Metadata reconciliation can validate a future pending source without granting
// execution permission or rendering an early recipient message.
func (s Service) resolveScope(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
	execution bool,
) (communications.ScopeAudience, error) {
	result := communications.ScopeAudience{}
	if scope.OccurrenceID == uuid.Nil {
		return result, access.ErrInvalid
	}
	basis, err := s.loadScope(ctx, tx, eventID, campaignID, version, scope, execution)
	if err != nil {
		return result, err
	}
	q := dbgen.New(tx)
	event, row, binding := basis.Event, basis.Occurrence, basis.Binding
	source := Source{
		Kind:       row.SourceKind,
		ID:         row.SourceID,
		Revision:   row.SourceRevision,
		OccurredAt: row.DueAt,
	}
	// Scope identity binds the immutable source key, never the changing audience.
	if row.ScopeBinding != scopeBinding(binding, source, row.ReviewRevision) {
		return result, communications.ErrUnresolved
	}
	var audience SourceAudience
	if row.SourceKind == "schedule" {
		if source.ID != binding.Definition.ID || source.Revision != binding.Version ||
			!row.DueAt.Equal(binding.DueAt) {
			return result, communications.ErrUnresolved
		}
		audience.RecipientIDs, err = commonRecipients(ctx, q, eventID, s.now())
		audience.Broadcast = true
	} else {
		audience, err = s.sourcePolicy(binding.Definition.Trigger).
			ResolveInTransaction(ctx, tx, eventID, source)
	}
	if err != nil {
		return result, fmt.Errorf("recheck authoritative automation source: %w", err)
	}
	if len(audience.RecipientIDs) == 0 {
		return result, communications.ErrUnresolved
	}
	result = communications.ScopeAudience{
		Binding:         row.ScopeBinding,
		RecipientIDs:    audience.RecipientIDs,
		Broadcast:       audience.Broadcast,
		RequireComplete: audience.RequireComplete,
	}
	if binding.Definition.RetainAfterArchive {
		allowed, err := q.RetainedCommunicationOccurrence(
			ctx,
			dbgen.RetainedCommunicationOccurrenceParams{
				EventID:      eventID,
				OccurrenceID: row.ID,
				AtTime:       s.now(),
			},
		)
		if err != nil {
			return result, fmt.Errorf("recheck retained occurrence: %w", err)
		}
		if !allowed || !audience.RetainedFollowUp {
			return result, access.ErrForbidden
		}
		result.AllowArchived = true
		result.Approval = &communications.ScopeApproval{
			Kind:          "retained_followup",
			ID:            binding.Definition.ID,
			Binding:       binding.CampaignApproval,
			SourceID:      row.SourceID,
			SourceVersion: row.SourceRevision,
		}
	}
	if !event.WritesAllowed && (!result.AllowArchived || event.Stage != "archived") {
		return result, access.ErrForbidden
	}

	if binding.PublicationApprovalID != nil {
		approval, err := q.PublicationPlanApproval(
			ctx,
			dbgen.PublicationPlanApprovalParams{
				EventID: eventID,
				ID:      *binding.PublicationApprovalID,
			},
		)
		if err != nil {
			return result, fmt.Errorf("read scoped publication approval: %w", err)
		}
		var proof struct {
			Provenance string `json:"provenance"`
		}
		if err = json.Unmarshal(approval.Document, &proof); err != nil {
			return result, fmt.Errorf("decode scoped publication provenance: %w", err)
		}
		if !approval.Approved {
			return result, access.ErrForbidden
		}
		result.Approval = &communications.ScopeApproval{
			Kind:          "expected_publication",
			ID:            approval.ID,
			Binding:       proof.Provenance,
			SourceID:      approval.SourceID,
			SourceVersion: approval.SourceVersion,
		}
	}
	return result, nil
}

type scopeBasis struct {
	Event      dbgen.Event
	Occurrence dbgen.CommunicationOccurrence
	Binding    Binding
}

func (s Service) loadScope(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
	execution bool,
) (scopeBasis, error) {
	q := dbgen.New(tx)
	event, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return scopeBasis{}, fmt.Errorf("lock current automation scope: %w", err)
	}
	row, err := q.AutomationOccurrence(
		ctx,
		dbgen.AutomationOccurrenceParams{EventID: eventID, ID: scope.OccurrenceID},
	)
	if err != nil {
		return scopeBasis{}, fmt.Errorf("read current automation occurrence: %w", err)
	}
	if !slices.Contains([]string{"pending", "queued", "partially_queued"}, row.State) {
		return scopeBasis{}, communications.ErrUnresolved
	}
	paused, err := q.AutomationPaused(ctx, eventID)
	if err != nil {
		return scopeBasis{}, fmt.Errorf("read current automation pause: %w", err)
	}
	if paused {
		return scopeBasis{}, communications.ErrUnresolved
	}
	binding, err := loadBinding(ctx, q, eventID, row.BindingID)
	if err != nil {
		return scopeBasis{}, err
	}
	if binding.Cancelled || binding.ApprovedAt == nil || binding.Version != row.BindingVersion ||
		binding.Definition.CampaignID != campaignID ||
		binding.Definition.CampaignVersion != version {
		return scopeBasis{}, communications.ErrUnresolved
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		binding.ActorID,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return scopeBasis{}, fmt.Errorf("recheck automation approver: %w", err)
	}
	campaign, err := currentCampaign(ctx, q, eventID, campaignID)
	if err != nil {
		return scopeBasis{}, err
	}
	if campaign.Cancelled || campaign.Version != version {
		return scopeBasis{}, communications.ErrUnresolved
	}
	if binding.PublicationApprovalID == nil &&
		(campaign.Approval == nil || campaign.Approval.Binding != binding.CampaignApproval) {
		return scopeBasis{}, communications.ErrUnresolved
	}
	deadline := row.Deadline
	if row.ReviewUntil.Valid {
		deadline = row.ReviewUntil.Time
	}
	if execution && (s.now().Before(row.DueAt) || s.now().After(deadline)) {
		return scopeBasis{}, communications.ErrUnresolved
	}
	return scopeBasis{Event: event, Occurrence: row, Binding: binding}, nil
}
