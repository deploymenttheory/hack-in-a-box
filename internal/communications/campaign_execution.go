package communications

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) selfMessage(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Campaign,
	id uuid.UUID,
	channel string,
) (TestMessage, string, error) {
	t, a, err := loadPair(ctx, q, row.ID, c.Definition.TemplateID, c.Definition.AudienceID)
	if err != nil {
		return TestMessage{}, "", err
	}
	purpose, format := t.Purpose, t.Format
	providerRevision := ""
	for _, ready := range s.channelReadiness(ctx, row.ID, actor) {
		if ready.Channel == channel {
			providerRevision = ready.ConfigRevision
		}
	}
	// Rendering as literal text supplies the provider's plain MIME part. The
	// provider escapes that text for branded HTML; template markup stays inert.
	t.Format = "text"
	self := Audience{
		ID:                 uuid.New(),
		Name:               "Requesting organiser only",
		Roles:              []string{"organiser"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
		FactID:             a.FactID,
	}
	p, err := s.build(ctx, q, row, t, self)
	if err != nil {
		return TestMessage{}, "", err
	}
	index := slices.IndexFunc(p.Recipients, func(r Recipient) bool { return r.PersonID == actor })
	if index < 0 || len(p.Recipients[index].Unresolved) > 0 {
		return TestMessage{}, "", blocked("self_test_source_unavailable")
	}
	recipient := p.Recipients[index]
	message := TestMessage{
		ID:               id,
		EventID:          row.ID,
		ActorID:          actor,
		Channel:          channel,
		Purpose:          purpose,
		Format:           format,
		Subject:          "[TEST] " + recipient.Subject,
		PlainText:        "This is a test for the requesting organiser only.\n\n" + recipient.Body,
		IncludeCalendar:  c.Definition.IncludeCalendar,
		ProviderRevision: providerRevision,
	}
	fingerprint := digest(struct {
		TemplateVersion, AudienceVersion int64
		Facts                            string
		Message                          TestMessage
	}{t.Version, a.Version, p.Fingerprint, message})
	return message, fingerprint, nil
}

func (s Service) queueTestSelf(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Campaign,
	input CampaignCommand,
) (*uuid.UUID, error) {
	if c.Cancelled || !slices.Contains(c.Definition.Channels, input.Channel) ||
		input.Channel == "teams" {
		return nil, blocked("self_test_channel_unavailable")
	}
	ready := s.channelReadiness(ctx, row.ID, actor)
	if s.Jobs == nil ||
		slices.IndexFunc(
			ready,
			func(r ChannelReadiness) bool { return r.Channel == input.Channel && r.SelfTestAvailable },
		) < 0 {
		return nil, blocked("self_test_provider_unavailable")
	}
	id := uuid.New()
	message, fingerprint, err := s.selfMessage(ctx, q, row, actor, c, id, input.Channel)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, contextError("encode organiser test", err)
	}
	if err = q.SaveCommunicationTestRequest(
		ctx,
		dbgen.SaveCommunicationTestRequestParams{
			EventID:         row.ID,
			ID:              id,
			CampaignID:      c.Definition.ID,
			CampaignVersion: c.Version,
			ActorID:         actor,
			Channel:         input.Channel,
			Snapshot:        raw,
			Fingerprint:     fingerprint,
			Reason:          input.Reason,
		},
	); err != nil {
		return nil, contextError("record organiser test", err)
	}
	_, err = s.Jobs.Enqueue(
		ctx,
		tx,
		dbgen.InsertOperationParams{
			EventID:      row.ID,
			OperationKey: "communication_test:" + id.String() + ":" + input.Channel,
			Kind:         "communication_test_" + input.Channel,
			TargetID:     id,
			DueAt:        s.now(),
			Approved:     true,
		},
	)
	if err != nil {
		return nil, contextError("enqueue organiser test", err)
	}
	return &id, nil
}

// ResolveTestSelfInTransaction rechecks the fixed requester, source and campaign
// before a provider records its attempt. It never resolves a caller-chosen
// address or a shared Teams channel, and never claims provider acceptance.
func (s Service) ResolveTestSelfInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, testID uuid.UUID,
) (TestMessage, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return TestMessage{}, contextError("lock test event", err)
	}
	request, err := q.CommunicationTestRequest(
		ctx,
		dbgen.CommunicationTestRequestParams{EventID: eventID, ID: testID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return TestMessage{}, access.ErrNotFound
	}
	if err != nil {
		return TestMessage{}, contextError("read organiser test", err)
	}
	if err = authorize(ctx, q, request.ActorID, eventID, true); err != nil {
		return TestMessage{}, err
	}
	c, err := loadCampaign(ctx, q, eventID, request.CampaignID)
	if err != nil {
		return TestMessage{}, err
	}
	if c.Cancelled || c.Version != request.CampaignVersion || request.Channel == "teams" {
		return TestMessage{}, ErrUnresolved
	}
	readiness := s.channelReadiness(ctx, eventID, request.ActorID)
	if slices.IndexFunc(
		readiness,
		func(r ChannelReadiness) bool { return r.Channel == request.Channel && r.SelfTestAvailable },
	) < 0 {
		return TestMessage{}, ErrUnresolved
	}
	message, fingerprint, err := s.selfMessage(
		ctx,
		q,
		row,
		request.ActorID,
		c,
		testID,
		request.Channel,
	)
	if err != nil {
		return TestMessage{}, err
	}
	if fingerprint != request.Fingerprint {
		return TestMessage{}, ErrUnresolved
	}
	return message, nil
}

// ResolveApprovedCampaignInTransaction supplies #82 with the currently eligible
// recipients and fully bound definition. The caller owns missed-action policy,
// event-wide pause, recipient attempt deduplication and unattended execution.
func (s Service) ResolveApprovedCampaignInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
) (ApprovedCampaign, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return ApprovedCampaign{}, contextError("lock approved campaign", err)
	}
	c, err := loadCampaign(ctx, q, eventID, id)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	if c.Cancelled || c.Approval == nil {
		return ApprovedCampaign{}, ErrUnresolved
	}
	if err = authorize(ctx, q, c.Approval.ActorID, eventID, true); err != nil {
		return ApprovedCampaign{}, err
	}
	basis, err := s.campaignBasis(ctx, q, row, c.Approval.ActorID)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	view := campaignView(c, basis)
	if view.Status != "approved" || view.DueAt == nil || view.DueAt.After(s.now()) {
		return ApprovedCampaign{}, ErrUnresolved
	}
	preview, err := s.ResolveApprovedInTransaction(ctx, tx, eventID, c.Approval.PreviewID)
	if err != nil {
		return ApprovedCampaign{}, err
	}
	template, _, err := basis.pair(c.Definition)
	if err != nil {
		return ApprovedCampaign{}, err
	}

	return ApprovedCampaign{
		Approval:   *c.Approval,
		Purpose:    template.Purpose,
		Format:     template.Format,
		Definition: c.Definition,
		Version:    c.Version,
		DueAt:      *view.DueAt,
		Preview:    preview,
	}, nil
}

func (s Service) ResolveApprovedBroadcastInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
) (BroadcastMessage, error) {
	approved, err := s.ResolveApprovedCampaignInTransaction(ctx, tx, eventID, campaignID)
	if err != nil {
		return BroadcastMessage{}, err
	}
	if !slices.Contains(approved.Definition.Channels, "teams") {
		return BroadcastMessage{}, access.ErrForbidden
	}
	q := dbgen.New(tx)
	row, err := q.Event(ctx, eventID)
	if err != nil {
		return BroadcastMessage{}, contextError("read broadcast event", err)
	}
	template, audience, err := loadPair(
		ctx,
		q,
		eventID,
		approved.Definition.TemplateID,
		approved.Definition.AudienceID,
	)
	if err != nil {
		return BroadcastMessage{}, err
	}
	facts, err := q.CommunicationFacts(ctx, eventID)
	if err != nil {
		return BroadcastMessage{}, contextError("read broadcast facts", err)
	}
	if len(broadcastBlockers(template, audience, facts)) > 0 {
		return BroadcastMessage{}, ErrUnresolved
	}
	template.Format = "text"
	projection, err := s.build(ctx, q, row, template, audience)
	if err != nil {
		return BroadcastMessage{}, err
	}
	if len(projection.Recipients) == 0 || len(projection.Blockers) > 0 {
		return BroadcastMessage{}, ErrUnresolved
	}
	first := projection.Recipients[0]
	for _, recipient := range projection.Recipients {
		if recipient.Subject != first.Subject || recipient.Body != first.Body ||
			len(recipient.Unresolved) > 0 {
			return BroadcastMessage{}, ErrUnresolved
		}
	}
	return BroadcastMessage{
		EventID:         eventID,
		CampaignID:      campaignID,
		CampaignVersion: approved.Version,
		Approval:        approved.Approval,
		DueAt:           approved.DueAt,
		Purpose:         approved.Purpose,
		Subject:         first.Subject,
		PlainText:       first.Body,
	}, nil
}
