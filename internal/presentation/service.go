package presentation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

type Service struct {
	Results        ResultAuthority
	OrderAuthority JudgeOrderAuthority
	Pool           *pgxpool.Pool
	Clock          func() time.Time
}

// ResultAuthority supplies only explicitly announced, safe room projections.
// Pairing validation stays with presentation and never uses controller roles.
type ResultAuthority interface {
	RoomAnnouncementsInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
	) ([]results.Announcement, error)
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func controller(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID) error {
	p, err := q.Person(ctx, actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return access.ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("read presentation identity: %w", err)
	}
	if !p.Active {
		return access.ErrForbidden
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return fmt.Errorf("read presentation authority: %w", err)
	}
	if !access.HasPresentationAuthority(roles) {
		return access.ErrForbidden
	}
	return nil
}

func locked(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("lock presentation event: %w", err)
	}
	return row, nil
}

func state(ctx context.Context, q *dbgen.Queries, id uuid.UUID) (dbgen.PresentationState, error) {
	if err := q.EnsurePresentation(ctx, id); err != nil {
		return dbgen.PresentationState{}, fmt.Errorf("initialise presentation: %w", err)
	}
	row, err := q.PresentationState(ctx, id)
	if err != nil {
		return row, fmt.Errorf("read presentation state: %w", err)
	}
	return row, nil
}

func decode(row dbgen.PresentationState) ([]Cue, []Cue, error) {
	draft, published := []Cue{}, []Cue{}
	if err := json.Unmarshal(row.Draft, &draft); err != nil {
		return nil, nil, fmt.Errorf("decode cue draft: %w", err)
	}
	if err := json.Unmarshal(row.Published, &published); err != nil {
		return nil, nil, fmt.Errorf("decode approved cues: %w", err)
	}
	return draft, published, nil
}

func (s Service) page(
	ctx context.Context,
	q *dbgen.Queries,
	eventRow dbgen.Event,
	row dbgen.PresentationState,
) (Page, error) {
	draft, published, err := decode(row)
	if err != nil {
		return Page{}, err
	}
	c := event.DefaultConfiguration()
	if err = json.Unmarshal(eventRow.Configuration, &c); err != nil {
		return Page{}, fmt.Errorf("decode cue timezone: %w", err)
	}
	override, overridden, err := decodeOverride(row)
	if err != nil {
		return Page{}, err
	}
	result := Page{
		Revision:         row.Revision,
		ServerNow:        s.now(),
		Timezone:         c.Timezone,
		Draft:            draft,
		Published:        published,
		CurrentIndex:     int(row.CurrentIndex),
		Mode:             row.Mode,
		RemainingSeconds: int(remaining(row, s.now())),
		Pairings:         []Pairing{},
	}
	result.CanControl = eventRow.WritesAllowed
	result.OverrideMode = row.OverrideMode
	result.OverrideRemainingSeconds = int(overrideRemaining(row, s.now()))
	if overridden {
		result.Override = &override
	}
	if row.OverrideTimerEndsAt.Valid {
		result.OverrideTimerEndsAt = &row.OverrideTimerEndsAt.Time
	}
	if row.TimerEndsAt.Valid {
		result.TimerEndsAt = &row.TimerEndsAt.Time
	}
	pairings, err := q.DisplayPairings(ctx, eventRow.ID)
	if err != nil {
		return Page{}, fmt.Errorf("list display pairings: %w", err)
	}
	for _, p := range pairings {
		result.Pairings = append(result.Pairings, pairingMetadata(p, s.now()))
	}

	return result, nil
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := locked(ctx, q, eventID)
		if err != nil {
			return err
		}
		if err = controller(ctx, q, actor, eventID); err != nil {
			return err
		}
		row, err := state(ctx, q, eventID)
		if err != nil {
			return err
		}
		result, err = s.page(ctx, q, e, row)
		return err
	})
	if err != nil {
		return Page{}, fmt.Errorf("read presentation: %w", err)
	}
	return result, nil
}

func save(ctx context.Context, q *dbgen.Queries, row dbgen.PresentationState) error {
	err := q.SavePresentationState(
		ctx,
		dbgen.SavePresentationStateParams(row),
	)
	if err != nil {
		return fmt.Errorf("save presentation: %w", err)
	}
	return nil
}

func record(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, target uuid.UUID,
	action audit.Action,
	revision int64,
) error {
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			EventID:  &eventID,
			TargetID: target,
			Action:   action,
			Reason:   audit.OrganiserDecision,
			Metadata: audit.Metadata{Revision: &revision},
		},
	); err != nil {
		return fmt.Errorf("record presentation audit: %w", err)
	}
	return nil
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	c Command,
) (Page, error) {
	if err := validate(c); err != nil {
		return Page{}, err
	}
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := locked(ctx, q, eventID)
		if err != nil {
			return err
		}
		if err = controller(ctx, q, actor, eventID); err != nil {
			return err
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		row, err := state(ctx, q, eventID)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if err = change(&row, c, s.now()); err != nil {
			return err
		}
		row.Revision++
		if err = save(ctx, q, row); err != nil {
			return err
		}
		if err = record(
			ctx,
			q,
			actor,
			eventID,
			eventID,
			audit.PresentationChanged,
			row.Revision,
		); err != nil {
			return err
		}
		if _, err = q.RecordEventDecision(
			ctx,
			dbgen.RecordEventDecisionParams{
				EventID:  eventID,
				ActorID:  actor,
				Kind:     "presentation." + c.Action,
				Reason:   c.Reason,
				Revision: e.Revision,
			},
		); err != nil {
			return fmt.Errorf("record cue decision: %w", err)
		}
		result, err = s.page(ctx, q, e, row)
		return err
	})
	if err != nil {
		return Page{}, fmt.Errorf("change presentation: %w", err)
	}
	return result, nil
}

func (s Service) Pair(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input PairInput,
) (Paired, error) {
	if strings.TrimSpace(input.Label) == "" || len(input.Label) > 100 || input.Minutes < 5 ||
		input.Minutes > 1440 ||
		strings.TrimSpace(input.Reason) == "" ||
		len(input.Reason) > 2000 {
		return Paired{}, access.ErrInvalid
	}
	var result Paired
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := locked(ctx, q, eventID)
		if err != nil {
			return err
		}
		if err = controller(ctx, q, actor, eventID); err != nil {
			return err
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		row, err := state(ctx, q, eventID)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		secret := rand.Text() + rand.Text()
		digest := sha256.Sum256([]byte(secret))
		now := s.now()
		id := uuid.New()
		expires := now.Add(time.Duration(input.Minutes) * time.Minute)
		if err = q.CreateDisplayPairing(
			ctx,
			dbgen.CreateDisplayPairingParams{
				EventID:   eventID,
				ID:        id,
				Label:     input.Label,
				TokenHash: digest[:],
				IssuerID:  actor,
				CreatedAt: now,
				ExpiresAt: expires,
			},
		); err != nil {
			return fmt.Errorf("create display pairing: %w", err)
		}
		row.Revision++
		if err = save(ctx, q, row); err != nil {
			return err
		}
		if err = record(ctx, q, actor, eventID, id, audit.DisplayPaired, row.Revision); err != nil {
			return err
		}
		if err = decision(ctx, q, e, actor, "display.paired", input.Reason); err != nil {
			return err
		}
		result = Paired{
			Revision: row.Revision,
			Pairing: Pairing{
				ID:        id,
				Label:     input.Label,
				CreatedAt: now,
				ExpiresAt: expires,
				Stale:     true,
			},
			Token: secret,
		}
		return nil
	})
	if err != nil {
		return Paired{}, fmt.Errorf("pair display: %w", err)
	}
	return result, nil
}

func (s Service) Revoke(
	ctx context.Context,
	actor, eventID, pairingID uuid.UUID,
	revision int64,
	reason string,
) (Page, error) {
	if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return Page{}, access.ErrInvalid
	}
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := locked(ctx, q, eventID)
		if err != nil {
			return err
		}
		if err = controller(ctx, q, actor, eventID); err != nil {
			return err
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		row, err := state(ctx, q, eventID)
		if err != nil {
			return err
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		affected, err := q.RevokeDisplayPairing(
			ctx,
			dbgen.RevokeDisplayPairingParams{
				EventID:   eventID,
				ID:        pairingID,
				RevokedAt: pgtype.Timestamptz{Time: s.now(), Valid: true},
			},
		)
		if err != nil {
			return fmt.Errorf("revoke display pairing: %w", err)
		}
		if affected == 0 {
			return access.ErrNotFound
		}
		row.Revision++
		if err = save(ctx, q, row); err != nil {
			return err
		}
		if err = record(
			ctx,
			q,
			actor,
			eventID,
			pairingID,
			audit.DisplayRevoked,
			row.Revision,
		); err != nil {
			return err
		}
		if err = decision(ctx, q, e, actor, "display.revoked", reason); err != nil {
			return err
		}
		result, err = s.page(ctx, q, e, row)
		return err
	})
	if err != nil {
		return Page{}, fmt.Errorf("revoke display: %w", err)
	}
	return result, nil
}

func (s Service) Snapshot(ctx context.Context, eventID uuid.UUID, token string) (Snapshot, error) {
	if len(token) < 32 || len(token) > 200 {
		return Snapshot{}, access.ErrUnauthenticated
	}
	if s.Pool == nil {
		return Snapshot{}, database.ErrDatabase
	}
	var result Snapshot
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		e, err := s.displayEvent(ctx, q, eventID, token)
		if err != nil {
			return err
		}
		row, err := state(ctx, q, eventID)
		if err != nil {
			return err
		}
		result, err = s.programSnapshot(ctx, q, e, row)
		if err != nil {
			return err
		}
		result.Results = []results.Announcement{}
		if s.Results != nil {
			result.Results, err = s.Results.RoomAnnouncementsInTransaction(ctx, tx, eventID)
			if err != nil {
				return fmt.Errorf("read announced room results: %w", err)
			}
		}
		if err = displayBranding(ctx, q, eventID, &result); err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(token))
		if err = q.TouchDisplayPairing(
			ctx,
			dbgen.TouchDisplayPairingParams{
				EventID:        eventID,
				TokenHash:      digest[:],
				SeenAt:         s.now(),
				ThrottleBefore: s.now().Add(-5 * time.Second),
			},
		); err != nil {
			return fmt.Errorf("record display heartbeat: %w", err)
		}
		return nil
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("display snapshot: %w", err)
	}
	return result, nil
}

func displayBranding(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	result *Snapshot,
) error {
	brand, err := templates.RenderBranding(ctx, q, eventID)
	if err != nil {
		return fmt.Errorf("read display branding: %w", err)
	}
	result.PrimaryColour, result.BackgroundColour, result.ForegroundColour = brand.Primary, brand.Background, brand.Foreground
	result.Assets = []DisplayAsset{}
	for _, asset := range brand.Assets {
		digest := sha256.Sum256(asset.Content)
		result.Assets = append(
			result.Assets,
			DisplayAsset{
				ID:        asset.ID,
				Kind:      asset.Kind,
				MediaType: asset.MediaType,
				Digest:    fmt.Sprintf("%x", digest),
			},
		)
	}

	return nil
}

func (s Service) project(
	ctx context.Context,
	q *dbgen.Queries,
	e dbgen.Event,
	c Cue,
) (DisplayCue, bool, error) {
	result := DisplayCue{ID: c.ID, Title: c.Title, Text: c.Text, URL: c.URL}
	if c.ContentID == nil {
		return result, true, nil
	}
	item, err := q.ContentItem(ctx, dbgen.ContentItemParams{EventID: e.ID, ID: *c.ContentID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DisplayCue{}, false, nil
	}
	if err != nil {
		return DisplayCue{}, false, fmt.Errorf("read cue content: %w", err)
	}
	if !item.ReleasedVersion.Valid || !item.ReleasedAt.Valid ||
		s.now().Before(item.ReleasedAt.Time) ||
		item.State == "withdrawn" {
		return DisplayCue{}, false, nil
	}
	if item.Kind == "challenge" && (!e.PublishedAt.Valid || s.now().Before(e.PublishedAt.Time)) {
		return DisplayCue{}, false, nil
	}
	version, err := q.ContentVersion(
		ctx,
		dbgen.ContentVersionParams{
			EventID:   e.ID,
			ContentID: item.ID,
			Version:   item.ReleasedVersion.Int64,
		},
	)
	if err != nil {
		return DisplayCue{}, false, fmt.Errorf("read released cue content: %w", err)
	}
	var document content.Document
	if err = json.Unmarshal(version.Document, &document); err != nil {
		return DisplayCue{}, false, fmt.Errorf("decode released cue content: %w", err)
	}
	if !document.Audience.Confirmed || len(document.Audience.Roles) > 0 ||
		len(document.Audience.Teams) > 0 {
		return DisplayCue{}, false, nil
	}
	result.ContentTitle = document.Title
	return result, true, nil
}

func decision(
	ctx context.Context,
	q *dbgen.Queries,
	e dbgen.Event,
	actor uuid.UUID,
	kind, reason string,
) error {
	if _, err := q.RecordEventDecision(
		ctx,
		dbgen.RecordEventDecisionParams{
			EventID:  e.ID,
			ActorID:  actor,
			Kind:     kind,
			Reason:   reason,
			Revision: e.Revision,
		},
	); err != nil {
		return fmt.Errorf("record display decision: %w", err)
	}
	return nil
}

func (s Service) displayEvent(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	token string,
) (dbgen.Event, error) {
	e, err := locked(ctx, q, eventID)
	if errors.Is(err, access.ErrNotFound) {
		return dbgen.Event{}, access.ErrUnauthenticated
	}
	if err != nil {
		return dbgen.Event{}, err
	}
	digest := sha256.Sum256([]byte(token))
	pairing, err := q.DisplayPairingByHash(
		ctx,
		dbgen.DisplayPairingByHashParams{EventID: eventID, TokenHash: digest[:]},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Event{}, access.ErrUnauthenticated
	}
	if err != nil {
		return dbgen.Event{}, fmt.Errorf("read display capability: %w", err)
	}
	if pairing.RevokedAt.Valid || !s.now().Before(pairing.ExpiresAt) ||
		!e.WritesAllowed {
		return dbgen.Event{}, access.ErrUnauthenticated
	}
	if err = controller(ctx, q, pairing.IssuerID, eventID); err != nil {
		if errors.Is(err, access.ErrForbidden) {
			return dbgen.Event{}, access.ErrUnauthenticated
		}
		return dbgen.Event{}, err
	}
	return e, nil
}

func pairingMetadata(p dbgen.DisplayPairingsRow, now time.Time) Pairing {
	v := Pairing{
		ID:        p.ID,
		Label:     p.Label,
		CreatedAt: p.CreatedAt,
		ExpiresAt: p.ExpiresAt,
		Stale:     true,
	}
	if p.RevokedAt.Valid {
		v.RevokedAt = &p.RevokedAt.Time
	}
	if p.LastSeenAt.Valid {
		v.LastSeenAt = &p.LastSeenAt.Time
		v.Stale = now.Before(p.LastSeenAt.Time) || now.Sub(p.LastSeenAt.Time) > 10*time.Second
	}
	return v
}

// Asset reads only a current approved branding asset after rechecking the event's
// display capability. An asset ID is never interpreted as an upload/file ID.
func (s Service) Asset(
	ctx context.Context,
	eventID uuid.UUID,
	token string,
	assetID uuid.UUID,
) (templates.Asset, error) {
	if s.Pool == nil {
		return templates.Asset{}, database.ErrDatabase
	}
	var result templates.Asset
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := s.displayEvent(ctx, q, eventID, token); err != nil {
			return err
		}
		brand, err := templates.RenderBranding(ctx, q, eventID)
		if err != nil {
			return fmt.Errorf("read display asset branding: %w", err)
		}
		for _, asset := range brand.Assets {
			if asset.ID == assetID {
				result = asset
				return nil
			}
		}
		return access.ErrNotFound
	})
	if err != nil {
		return templates.Asset{}, fmt.Errorf("read display asset: %w", err)
	}
	return result, nil
}
