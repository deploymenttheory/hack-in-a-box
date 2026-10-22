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

func occurrence(row dbgen.CommunicationOccurrence) Occurrence {
	o := Occurrence{
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
	if row.ReviewUntil.Valid {
		o.ReviewUntil = &row.ReviewUntil.Time
	}
	return o
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
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
	bindings, err := q.AutomationBindings(ctx, eventID)
	if err != nil {
		return result, fmt.Errorf("read automation bindings: %w", err)
	}
	if len(bindings) > 1000 {
		return result, access.ErrInvalid
	}
	for _, row := range bindings {
		var b Binding
		if err = json.Unmarshal(row.Document, &b); err != nil {
			return result, fmt.Errorf("decode automation status: %w", err)
		}
		result.Bindings = append(result.Bindings, b)
	}
	rows, err := q.AutomationOccurrences(ctx, eventID)
	if err != nil {
		return result, fmt.Errorf("read automation occurrences: %w", err)
	}
	for _, row := range rows {
		result.Occurrences = append(result.Occurrences, occurrence(row))
	}
	return result, nil
}
