package sessionbooking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type NotificationConfiguration struct {
	ID       uuid.UUID `json:"id"`
	Version  int64     `json:"version"`
	Channels []string  `json:"channels"`
	Approved bool      `json:"approved"`
}

type NotificationPreview struct {
	Configuration     NotificationConfiguration         `json:"configuration"`
	SessionRevision   int64                             `json:"session_revision"`
	CurrentRecipients int                               `json:"current_recipients"`
	Providers         []communications.ChannelReadiness `json:"providers"`
	CanApprove        bool                              `json:"can_approve"`
	Fingerprint       string                            `json:"fingerprint"`
}

type notificationConfiguration struct {
	NotificationConfiguration
	Actor     uuid.UUID
	Revisions map[string]string
	Binding   string
}

func hash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func notificationConfig(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
) (notificationConfiguration, error) {
	var result notificationConfiguration
	result.Channels = []string{}
	result.Revisions = map[string]string{}
	var channels, revisions []byte
	err := tx.QueryRow(ctx, `SELECT c.id,c.version,c.channels,a.configuration_id IS NOT NULL,COALESCE(a.actor_id,'00000000-0000-0000-0000-000000000000'::uuid),COALESCE(a.provider_revisions,'{}'::jsonb),COALESCE(a.binding,'') FROM bookable_sessions s JOIN session_notification_configurations c ON c.event_id=s.event_id AND c.session_id=s.id AND c.version=s.notification_version LEFT JOIN session_notification_approvals a ON a.configuration_id=c.id WHERE s.event_id=$1 AND s.id=$2`, eventID, id).
		Scan(&result.ID, &result.Version, &channels, &result.Approved, &result.Actor, &revisions, &result.Binding)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, contextError("read notification configuration", err)
	}
	if err = json.Unmarshal(channels, &result.Channels); err != nil {
		return result, contextError("decode notification channels", err)
	}
	if err = json.Unmarshal(revisions, &result.Revisions); err != nil {
		return result, contextError("decode approved providers", err)
	}
	return result, nil
}

// SaveNotificationConfiguration creates a new immutable contract version. It is
// deliberately not approval. It selects only existing private delivery adapters;
// broadcast channels cannot carry a person's booking or calendar identity.
func (s Service) SaveNotificationConfiguration(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	channels []string,
) (NotificationPreview, error) {
	var result NotificationPreview
	if len(channels) > 2 {
		return result, access.ErrInvalid
	}
	seen := map[string]bool{}
	for _, channel := range channels {
		if !slices.Contains([]string{"in_app", "smtp"}, channel) || seen[channel] {
			return result, access.ErrInvalid
		}
		seen[channel] = true
	}
	if channels == nil {
		channels = []string{}
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if !st.organiser || !st.row.WritesAllowed {
			return access.ErrForbidden
		}
		if st.revision != expected {
			return access.ErrConflict
		}
		if err = s.checkNotificationEffects(ctx, tx, eventID, id); err != nil {
			return err
		}
		var version int64
		if err = tx.QueryRow(ctx, `UPDATE bookable_sessions SET notification_version=notification_version+1,revision=revision+1 WHERE event_id=$1 AND id=$2 RETURNING notification_version`, eventID, id).
			Scan(&version); err != nil {
			return contextError("version notification contract", err)
		}
		encoded, err := json.Marshal(channels)
		if err != nil {
			return contextError("encode notification contract", err)
		}
		if _, err = tx.Exec(
			ctx,
			`INSERT INTO session_notification_configurations(event_id,session_id,version,channels,actor_id) VALUES($1,$2,$3,$4,$5)`,
			eventID,
			id,
			version,
			encoded,
			actor,
		); err != nil {
			return contextError("retain notification contract", err)
		}
		config, err := notificationConfig(ctx, tx, eventID, id)
		if err != nil {
			return err
		}
		if err = installNotificationComposition(
			ctx,
			dbgen.New(tx),
			eventID,
			actor,
			config,
		); err != nil {
			return err
		}
		st, err = load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if err = history(
			ctx,
			tx,
			actor,
			eventID,
			id,
			nil,
			"save_notifications",
			st.revision,
		); err != nil {
			return err
		}
		result, _, err = s.previewNotifications(ctx, tx, actor, st)
		return err
	})
	return result, contextError("save notifications", err)
}

// Existing delivery tables retain an immutable campaign identity. This adapter
// records a composition only, with no generic campaign approval; the registered
// session owner alone supplies the separate native approval and exact recipient.
func installNotificationComposition(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, actor uuid.UUID,
	config notificationConfiguration,
) error {
	template := communications.Template{
		ID:      config.ID,
		Version: config.Version,
		Name:    "Native session notification",
		Purpose: "reminder",
		Format:  "text",
		Subject: "Session booking update",
		Body:    "Open your session booking for its current state.",
	}
	audience := communications.Audience{
		ID:                 config.ID,
		Version:            config.Version,
		Name:               "Native current own session booking",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for kind, document := range map[string]any{"template": template, "audience": audience} {
		raw, err := json.Marshal(document)
		if err != nil {
			return contextError("encode native notification composition", err)
		}
		if err = q.SaveCommunicationVersion(
			ctx,
			dbgen.SaveCommunicationVersionParams{
				EventID:  eventID,
				ID:       config.ID,
				Kind:     kind,
				Version:  config.Version,
				Document: raw,
				ActorID:  actor,
			},
		); err != nil {
			return contextError("retain native notification composition", err)
		}
	}
	campaign := communications.Campaign{
		Definition: communications.CampaignDefinition{
			ID:              config.ID,
			Name:            "Native session notification (separate approval)",
			TemplateID:      config.ID,
			AudienceID:      config.ID,
			Channels:        config.Channels,
			IncludeCalendar: slices.Contains(config.Channels, "smtp"),
			Timing:          communications.CampaignTiming{Kind: "relative", Anchor: "live_start"},
		},
		Version: config.Version,
	}
	raw, err := json.Marshal(campaign)
	if err != nil {
		return contextError("encode native notification identity", err)
	}
	if err = q.SaveCommunicationCampaign(
		ctx,
		dbgen.SaveCommunicationCampaignParams{
			EventID:  eventID,
			ID:       config.ID,
			Version:  config.Version,
			Document: raw,
			ActorID:  actor,
			Reason:   "Native session composition; generic campaign remains unapproved",
		},
	); err != nil {
		return contextError("retain native notification identity", err)
	}
	return nil
}

func (s Service) PreviewNotifications(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (NotificationPreview, error) {
	var result NotificationPreview
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if !st.organiser {
			return access.ErrForbidden
		}
		result, _, err = s.previewNotifications(ctx, tx, actor, st)
		return err
	})
	return result, contextError("preview notifications", err)
}

func (s Service) previewNotifications(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	st scope,
) (NotificationPreview, map[string]string, error) {
	config, err := notificationConfig(ctx, tx, st.row.ID, st.item.ID)
	if err != nil {
		return NotificationPreview{}, nil, err
	}
	result := NotificationPreview{
		Configuration:   config.NotificationConfiguration,
		SessionRevision: st.revision,
		Providers:       []communications.ChannelReadiness{},
		CanApprove:      st.row.WritesAllowed && config.ID != uuid.Nil && len(config.Channels) > 0,
	}
	current := map[string]string{}
	for _, channel := range config.Channels {
		ready := communications.ChannelReadiness{Channel: channel, Reason: "not_configured"}
		if provider := s.DeliveryChannels[channel]; provider != nil {
			ready, err = provider.Readiness(ctx, st.row.ID, actor)
			if err != nil {
				return result, nil, contextError("review notification provider", err)
			}
			ready.Channel = channel
		}
		result.Providers = append(result.Providers, ready)
		if !ready.Available || ready.ConfigRevision == "" {
			result.CanApprove = false
		} else {
			current[channel] = ready.ConfigRevision
		}
	}
	people, err := notificationRecipients(ctx, tx, st.row.ID, st.item.ID)
	if err != nil {
		return result, nil, err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM session_bookings b JOIN people p ON p.id=b.person_id AND p.active JOIN competition_claims c ON c.event_id=b.event_id AND c.person_id=b.person_id AND c.state='confirmed' WHERE b.event_id=$1 AND b.session_id=$2 AND b.state IN ('booked','waitlisted') AND ($3 OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=b.event_id AND m.person_id=b.person_id AND m.active AND m.role=ANY($4::text[])))`, st.row.ID, st.item.ID, slices.Contains(st.item.Audience, "all") || slices.Contains(st.item.Audience, "attendee"), st.item.Audience).
		Scan(&result.CurrentRecipients); err != nil {
		return result, nil, contextError("count current native notification cohort", err)
	}
	result.Fingerprint = hash(struct {
		Contract                 string
		ID, Actor                uuid.UUID
		Version, SessionRevision int64
		Channels                 []string
		Providers                map[string]string
		Recipients               []notificationRecipient
	}{"session-notification-contract-v1", config.ID, actor, config.Version, st.revision, config.Channels, current, people})
	return result, current, nil
}

// Approval authorizes the fixed native booking lifecycle contract and selected
// provider revisions. Every later occurrence still binds an exact own booking
// revision/current session source, with a separate minimal cancellation shape.
func (s Service) ApproveNotifications(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	version int64,
	fingerprint string,
) (NotificationPreview, error) {
	var result NotificationPreview
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if !st.organiser || !st.row.WritesAllowed {
			return access.ErrForbidden
		}
		if err = s.checkNotificationEffects(ctx, tx, eventID, id); err != nil {
			return err
		}
		preview, revisions, err := s.previewNotifications(ctx, tx, actor, st)
		if err != nil {
			return err
		}
		if !preview.CanApprove || preview.Configuration.Version != version ||
			preview.Fingerprint != fingerprint {
			return access.ErrConflict
		}
		encoded, err := json.Marshal(revisions)
		if err != nil {
			return contextError("encode approved notification providers", err)
		}
		binding := hash(struct {
			Contract      string
			Configuration uuid.UUID
			Version       int64
			Actor         uuid.UUID
			Providers     map[string]string
		}{"session-notification-contract-v1", preview.Configuration.ID, version, actor, revisions})
		tag, err := tx.Exec(
			ctx,
			`INSERT INTO session_notification_approvals(configuration_id,actor_id,provider_revisions,binding) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
			preview.Configuration.ID,
			actor,
			encoded,
			binding,
		)
		if err != nil {
			return contextError("approve notification contract", err)
		}
		retained, err := notificationConfig(ctx, tx, eventID, id)
		if err != nil {
			return err
		}
		if retained.Binding != binding {
			return access.ErrConflict
		}
		if tag.RowsAffected() == 1 {
			if err = history(
				ctx,
				tx,
				actor,
				eventID,
				id,
				nil,
				"approve_notifications",
				st.revision,
			); err != nil {
				return err
			}
		}
		if err = s.queueUpdates(ctx, tx, actor, st, nil); err != nil {
			return err
		}
		result, _, err = s.previewNotifications(ctx, tx, actor, st)
		return err
	})
	return result, contextError("approve notifications", err)
}

func (s Service) checkNotificationEffects(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
) error {
	for _, provider := range s.DeliveryChannels {
		if guard, ok := provider.(communications.NativeSessionEffectGuard); ok {
			if err := guard.CheckSessionNotificationApprovalInTransaction(
				ctx,
				tx,
				eventID,
				id,
			); err != nil {
				return contextError("retain unresolved original session effect", err)
			}
		}
	}
	return nil
}
