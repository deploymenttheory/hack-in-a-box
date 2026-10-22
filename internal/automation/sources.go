package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type builtinSource struct {
	service Service
	kind    string
}

func (b builtinSource) DiscoverInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
) ([]Source, error) {
	result := []Source{}
	after := Source{}
	for {
		page, err := b.discoverPage(ctx, tx, eventID, d, after, nil)
		if err != nil {
			return nil, err
		}
		result = append(result, page...)
		if len(page) < 200 {
			return result, nil
		}
		after = page[len(page)-1]
	}
}

// DiscoverUnknownInTransaction advances through the durable set difference for
// this exact reviewed binding version. Starting at the lowest remaining key on
// every pass also catches late inserts whose UUID sorts before a previous page.
func (b builtinSource) DiscoverUnknownInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	binding Binding,
) ([]Source, error) {
	return b.discoverPage(ctx, tx, eventID, binding.Definition, Source{}, &binding)
}

func (b builtinSource) discoverPage(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	d Definition,
	after Source,
	binding *Binding,
) ([]Source, error) {
	var sourceQuery string
	switch b.kind {
	case "registration.offer":
		sourceQuery = `SELECT f.id,1::bigint,o.due_at FROM registration_offers f JOIN operations o ON o.event_id=f.event_id AND o.target_id=f.id AND o.kind='registration.offer' WHERE f.event_id=$1`
	case "registration.place_changed":
		sourceQuery = `SELECT h.id,1::bigint,h.created_at FROM registration_history h JOIN registrations r ON r.event_id=h.event_id AND r.id=h.registration_id WHERE h.event_id=$1 AND r.kind='attendee' AND h.from_state<>h.to_state`
	case "facilitator.selected":
		sourceQuery = `SELECT h.id,1::bigint,h.created_at FROM registration_history h JOIN registrations r ON r.event_id=h.event_id AND r.id=h.registration_id WHERE h.event_id=$1 AND r.kind='facilitator' AND h.to_state='selected'`
	case "teams.published", "mentors.published", "event.rescheduled":
		sourceQuery = `SELECT id,revision,due_at FROM operations WHERE event_id=$1 AND kind=$2`
	case "challenge.released", "brief.released":
		sourceQuery = `SELECT c.content_id,c.revision,c.occurred_at FROM content_changes c JOIN content_items i ON i.event_id=c.event_id AND i.id=c.content_id WHERE c.event_id=$1 AND c.action='released' AND i.kind=split_part($2::text,'.',1) AND i.state='released' AND i.released_version=c.version AND i.revision=c.revision AND ($9::bigint=0 OR i.released_version=$9)`
	default:
		return nil, communications.ErrUnresolved
	}
	var bindingID *uuid.UUID
	var version int64
	if binding != nil {
		bindingID = &binding.Definition.ID
		version = binding.Version
	}
	query := `WITH source(id,revision,occurred_at) AS (` + sourceQuery + `)
 SELECT id,revision,min(occurred_at) AS occurred_at FROM source
 WHERE ($3::uuid IS NULL OR id=$3) AND ($9::bigint=0 OR $2::text IN ('challenge.released','brief.released') OR revision=$9) AND (id,revision)>($4::uuid,$5::bigint)
 AND ($7::uuid IS NULL OR NOT EXISTS(SELECT 1 FROM communication_occurrences co WHERE co.event_id=$1 AND co.binding_id=$7 AND co.binding_version=$8 AND co.source_kind=$2::text AND co.source_id=source.id AND co.source_revision=source.revision))
 GROUP BY id,revision ORDER BY id,revision LIMIT $6`
	rows, err := tx.Query(
		ctx,
		query,
		eventID,
		b.kind,
		d.SourceID,
		after.ID,
		after.Revision,
		200,
		bindingID,
		version,
		d.SourceVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("discover authoritative lifecycle source page: %w", err)
	}
	defer rows.Close()
	result := []Source{}
	for rows.Next() {
		source := Source{Kind: b.kind}
		if err = rows.Scan(&source.ID, &source.Revision, &source.OccurredAt); err != nil {
			return nil, fmt.Errorf("read lifecycle source metadata: %w", err)
		}
		result = append(result, source)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lifecycle source page: %w", err)
	}
	return result, nil
}

func (b builtinSource) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source Source,
) (SourceAudience, error) {
	result := SourceAudience{}
	q := dbgen.New(tx)
	row, err := q.Event(ctx, eventID)
	if err != nil {
		return result, fmt.Errorf("read lifecycle event: %w", err)
	}
	if source.Kind != b.kind {
		return result, communications.ErrUnresolved
	}
	switch b.kind {
	case "outcome.follow_up":
		result, err = b.resolveOutcome(ctx, tx, row, source)
	case "registration.offer":
		result, err = b.resolveOffer(ctx, tx, row, source)
	case "registration.place_changed", "facilitator.selected":
		result, err = b.resolveRegistration(ctx, tx, row, source)
	case "teams.published", "mentors.published", "event.rescheduled":
		result, err = b.resolvePublishedOperation(ctx, tx, row, source)
	case "challenge.released", "brief.released":
		result, err = b.resolveReleasedContent(ctx, tx, row, source)
	default:
		return result, communications.ErrUnresolved
	}
	if err != nil {
		return result, err
	}
	if len(result.RecipientIDs) == 0 || len(result.RecipientIDs) > 10000 {
		return result, communications.ErrUnresolved
	}
	slices.SortFunc(
		result.RecipientIDs,
		func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) },
	)
	return result, nil
}

func (b builtinSource) resolveOutcome(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	source Source,
) (SourceAudience, error) {
	result := SourceAudience{}
	var err error
	eventID := row.ID
	var owner uuid.UUID
	var valid bool
	err = tx.QueryRow(ctx, `SELECT owner_id,(kind='outcome_review' AND status='open' AND revision=$3) FROM communication_facts WHERE event_id=$1 AND id=$2 AND owner_id IS NOT NULL`, eventID, source.ID, source.Revision).
		Scan(&owner, &valid)
	if err != nil {
		return result, fmt.Errorf("recheck retained outcome source: %w", err)
	}
	if !valid {
		return result, communications.ErrUnresolved
	}
	result.RecipientIDs = []uuid.UUID{owner}
	result.RetainedFollowUp = true

	return result, nil
}

func (b builtinSource) resolveOffer(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	source Source,
) (SourceAudience, error) {
	result := SourceAudience{}
	var err error
	eventID := row.ID
	var person uuid.UUID
	var valid bool
	err = tx.QueryRow(ctx, `SELECT r.person_id,(f.state='pending' AND r.state='offered' AND f.expires_at>$3) FROM registration_offers f JOIN registrations r ON r.event_id=f.event_id AND r.id=f.registration_id WHERE f.event_id=$1 AND f.id=$2`, eventID, source.ID, b.service.now()).
		Scan(&person, &valid)
	if err != nil {
		return result, fmt.Errorf("recheck current offered place: %w", err)
	}
	var config event.Configuration
	if err = json.Unmarshal(row.Configuration, &config); err != nil {
		return result, fmt.Errorf("read final admission cutoff: %w", err)
	}
	if !valid || source.Revision != 1 || !b.service.now().Before(config.AdmissionCutoff) {
		return result, communications.ErrUnresolved
	}
	result.RecipientIDs = []uuid.UUID{person}

	return result, nil
}

func (b builtinSource) resolveRegistration(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	source Source,
) (SourceAudience, error) {
	result := SourceAudience{}
	var err error
	eventID := row.ID
	var person uuid.UUID
	var current bool
	err = tx.QueryRow(ctx, `SELECT r.person_id,(r.state=h.to_state AND (r.kind='attendee' OR r.state='selected')) FROM registration_history h JOIN registrations r ON r.event_id=h.event_id AND r.id=h.registration_id WHERE h.event_id=$1 AND h.id=$2`, eventID, source.ID).
		Scan(&person, &current)
	if err != nil {
		return result, fmt.Errorf("recheck current registration transition: %w", err)
	}
	if !current || source.Revision != 1 {
		return result, communications.ErrUnresolved
	}
	result.RecipientIDs = []uuid.UUID{person}

	return result, nil
}

func (b builtinSource) resolvePublishedOperation(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	source Source,
) (SourceAudience, error) {
	result := SourceAudience{}
	q := dbgen.New(tx)
	var err error
	eventID := row.ID
	var target uuid.UUID
	var revision int64
	var valid bool
	err = tx.QueryRow(ctx, `SELECT target_id,revision,(approved AND NOT cancelled) FROM operations WHERE event_id=$1 AND id=$2 AND kind=$3`, eventID, source.ID, b.kind).
		Scan(&target, &revision, &valid)
	if err != nil {
		return result, fmt.Errorf("recheck lifecycle operation: %w", err)
	}
	if !valid || revision != source.Revision {
		return result, communications.ErrUnresolved
	}
	if b.kind == "event.rescheduled" {
		var config event.Configuration
		if err = json.Unmarshal(row.Configuration, &config); err != nil {
			return result, fmt.Errorf("read revised dates: %w", err)
		}
		err = tx.QueryRow(ctx, `SELECT live_start=$3 AND live_end=$4 AND submission_cutoff=$5 FROM event_notices WHERE event_id=$1 AND id=$2`, eventID, target, config.LiveStart, config.LiveEnd, config.SubmissionCutoff).
			Scan(&valid)
		if err != nil {
			return result, fmt.Errorf("recheck approved revised dates: %w", err)
		}
		if !valid {
			return result, communications.ErrUnresolved
		}
		rows, err := tx.Query(
			ctx,
			`SELECT p.id FROM people p WHERE p.active AND (EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=$1 AND c.person_id=p.id AND c.state='confirmed') OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=$1 AND m.person_id=p.id AND m.active)) ORDER BY p.id`,
			eventID,
		)
		if err != nil {
			return result, fmt.Errorf("resolve revised-date audience: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err = rows.Scan(&id); err != nil {
				return result, fmt.Errorf("read revised-date recipient: %w", err)
			}
			result.RecipientIDs = append(result.RecipientIDs, id)
		}
		if err = rows.Err(); err != nil {
			return result, fmt.Errorf("iterate revised-date audience: %w", err)
		}
		result.RequireComplete = true
	} else {
		var latest uuid.UUID
		err = tx.QueryRow(ctx, `SELECT id FROM operations WHERE event_id=$1 AND kind=$2 AND approved AND NOT cancelled ORDER BY due_at DESC,id DESC LIMIT 1`, eventID, b.kind).
			Scan(&latest)
		if err != nil {
			return result, fmt.Errorf("read current published allocation: %w", err)
		}
		if latest != source.ID {
			return result, communications.ErrUnresolved
		}
		result.RecipientIDs, err = commonRecipients(ctx, q, eventID, b.service.now())
		if err != nil {
			return result, err
		}
	}
	result.Broadcast = true

	return result, nil
}

func (b builtinSource) resolveReleasedContent(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	source Source,
) (SourceAudience, error) {
	result := SourceAudience{}
	var err error
	eventID := row.ID
	q := dbgen.New(tx)
	item, err := q.ContentItem(ctx, dbgen.ContentItemParams{EventID: eventID, ID: source.ID})
	if err != nil {
		return result, fmt.Errorf("read released lifecycle source: %w", err)
	}
	if item.State != "released" || item.Revision != source.Revision || !item.ReleasedAt.Valid ||
		b.service.now().Before(item.ReleasedAt.Time) ||
		!row.PublishedAt.Valid ||
		b.service.now().Before(row.PublishedAt.Time) {
		return result, communications.ErrUnresolved
	}
	var released bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM content_changes WHERE event_id=$1 AND content_id=$2 AND revision=$3 AND action='released' AND version=$4 AND occurred_at=$5)`, eventID, item.ID, item.Revision, item.Version, item.ReleasedAt.Time).
		Scan(&released)
	if err != nil {
		return result, fmt.Errorf("verify actual guarded publication: %w", err)
	}
	if !released {
		return result, communications.ErrUnresolved
	}

	doc, err := q.ContentVersion(
		ctx,
		dbgen.ContentVersionParams{EventID: eventID, ContentID: item.ID, Version: item.Version},
	)
	if err != nil {
		return result, fmt.Errorf("read released audience policy: %w", err)
	}
	var document struct {
		Audience access.ContentAudience `json:"audience"`
	}
	if err = json.Unmarshal(doc.Document, &document); err != nil {
		return result, fmt.Errorf("decode released audience policy: %w", err)
	}
	people, err := q.CommunicationPeople(
		ctx,
		dbgen.CommunicationPeopleParams{EventID: eventID, At: b.service.now()},
	)
	if err != nil {
		return result, fmt.Errorf("resolve publication readers: %w", err)
	}
	for _, person := range people {
		advertised := document.Audience.Confirmed && person.CompetitionState == "confirmed" ||
			slices.ContainsFunc(
				person.Roles,
				func(role string) bool { return slices.Contains(document.Audience.Roles, role) },
			) ||
			person.CompetitionState == "confirmed" &&
				slices.Contains(document.Audience.Teams, person.TeamID)
		if !advertised {
			continue
		}
		version, err := access.ContentVersionForReader(ctx, q, person.ID, item, false)
		if err != nil {
			return result, fmt.Errorf("recheck current publication audience: %w", err)
		}
		if version == item.Version {
			result.RecipientIDs = append(result.RecipientIDs, person.ID)
		}
	}
	result.Broadcast = document.Audience.Confirmed && len(document.Audience.Teams) == 0 &&
		len(document.Audience.Roles) == 0
	result.RequireComplete = true
	return result, nil
}
