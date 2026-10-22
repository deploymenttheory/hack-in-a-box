package automation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func loadBinding(ctx context.Context, q *dbgen.Queries, eventID, id uuid.UUID) (Binding, error) {
	row, err := q.AutomationBinding(ctx, dbgen.AutomationBindingParams{EventID: eventID, ID: id})
	if err != nil {
		return Binding{}, fmt.Errorf("read automation binding: %w", err)
	}
	var binding Binding
	if err = json.Unmarshal(row.Document, &binding); err != nil {
		return Binding{}, fmt.Errorf("decode automation binding: %w", err)
	}
	if binding.Definition.ID != row.ID || binding.Version != row.Version ||
		binding.Definition.CampaignID != row.CampaignID ||
		binding.Definition.CampaignVersion != row.CampaignVersion {
		return Binding{}, access.ErrConflict
	}
	return binding, nil
}

func occurrence(row dbgen.CommunicationOccurrence) (Occurrence, error) {
	o := Occurrence{
		ReviewRevision: row.ReviewRevision,
		ChannelReports: map[string]ChannelReport{},
		ID:             row.ID,
		BindingID:      row.BindingID,
		BindingVersion: row.BindingVersion,
		SourceKind:     row.SourceKind,
		SourceID:       row.SourceID,
		SourceRevision: row.SourceRevision,
		DueAt:          row.DueAt,
		Deadline:       row.Deadline,
		State:          row.State,
		Reason:         row.Reason,
	}
	if err := json.Unmarshal(row.ChannelReports, &o.ChannelReports); err != nil {
		return Occurrence{}, fmt.Errorf("decode retained channel evidence: %w", err)
	}
	if row.ReviewUntil.Valid {
		o.ReviewUntil = &row.ReviewUntil.Time
	}
	return o, nil
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	return s.ReadPage(ctx, actor, eventID, uuid.Nil, uuid.Nil, 200)
}

// ReadPage uses independent exclusive UUID cursors; exhausted cursors can be
// retained while paging the other collection. Every request rechecks authority.
func (s Service) ReadPage(
	ctx context.Context,
	actor, eventID, bindingsAfter, occurrencesAfter uuid.UUID,
	limit int32,
) (Page, error) {
	if limit < 1 || limit > 200 {
		return Page{}, access.ErrInvalid
	}
	q := dbgen.New(s.Pool)
	result := Page{Bindings: []Binding{}, Occurrences: []Occurrence{}}
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return result, fmt.Errorf("authorize automation status: %w", err)
	}
	event, err := q.Event(ctx, eventID)
	if err != nil {
		return result, fmt.Errorf("read automation event: %w", err)
	}
	result.Revision = event.Revision
	result.Paused, err = q.AutomationPaused(ctx, eventID)
	if err != nil {
		return result, fmt.Errorf("read automation pause: %w", err)
	}
	bindings, err := q.AutomationBindingsPage(
		ctx,
		dbgen.AutomationBindingsPageParams{
			EventID:  eventID,
			AfterID:  bindingsAfter,
			PageSize: limit + 1,
		},
	)
	if err != nil {
		return result, fmt.Errorf("read automation bindings: %w", err)
	}
	if len(bindings) > int(limit) {
		bindings = bindings[:limit]
		id := bindings[len(bindings)-1].ID
		result.NextBindingCursor = &id
	}
	for _, row := range bindings {
		var b Binding
		if err = json.Unmarshal(row.Document, &b); err != nil {
			return result, fmt.Errorf("decode automation status: %w", err)
		}
		result.Bindings = append(result.Bindings, b)
	}
	rows, err := q.AutomationOccurrencesPage(
		ctx,
		dbgen.AutomationOccurrencesPageParams{
			EventID:  eventID,
			AfterID:  occurrencesAfter,
			PageSize: limit + 1,
		},
	)
	if err != nil {
		return result, fmt.Errorf("read automation occurrences: %w", err)
	}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		id := rows[len(rows)-1].ID
		result.NextOccurrenceCursor = &id
	}
	for _, row := range rows {
		current, decodeErr := occurrence(row)
		if decodeErr != nil {
			return result, decodeErr
		}
		result.Occurrences = append(result.Occurrences, current)
	}
	return result, nil
}
