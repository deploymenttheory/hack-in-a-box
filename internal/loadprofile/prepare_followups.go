package loadprofile

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
)

// PreparedFollowup records original native commands separately from the
// assigned owner's current, deliberately redacted reading. Approval is not
// evidence of dispatch, provider acceptance, delivery or an original receipt.
type PreparedFollowup struct {
	Source   outcomes.View     `json:"current_native_source"`
	Saved    outcomes.PlanView `json:"original_save_acknowledgement"`
	Approved outcomes.PlanView `json:"original_approval_acknowledgement"`
	Owner    outcomes.PlanView `json:"current_owner_redacted_read"`
}

// PrepareFollowup must run against the normal-wall production deployment,
// after warmup has chosen the actual future due instant. Neither this helper
// nor its caller may advance a clock to make the reminder due.
func (p *NativePreparer) PrepareFollowup(
	ctx context.Context,
	organiser, owner *Session,
	outcomeID uuid.UUID,
	due time.Time,
) (PreparedFollowup, error) {
	var result PreparedFollowup
	if organiser == nil || owner == nil || organiser.Person() == owner.Person() ||
		outcomeID == uuid.Nil || due.IsZero() || !due.Equal(due.UTC().Truncate(time.Second)) {
		return result, fmt.Errorf(
			"%w: distinct native organiser/owner and explicit future due required",
			errCohort,
		)
	}
	path := "/api/v1/outcomes/" + outcomeID.String()
	source, err := prepareCall[outcomes.View](ctx, p, organiser, http.MethodGet, path, nil, nil)
	if err != nil {
		return result, err
	}
	result.Source = source
	if source.ID != outcomeID || source.OwnerID == nil || *source.OwnerID != owner.Person() ||
		(source.Status != "accepted" && source.Status != "in_progress") || source.Version.Version < 1 {
		return result, fmt.Errorf(
			"%w: actual accepted follow-through source/owner required",
			errCohort,
		)
	}
	id := uuid.New()
	reviewPath := path + "/reviews/" + id.String()
	schedule := outcomes.ReviewSchedule{
		Days: 30, WallTime: due.UTC().Format("2006-01-02T15:04:05"),
		Timezone: "UTC", Channels: []string{"in_app"},
	}
	message := outcomes.ReviewMessage{
		Subject: "Reviewed synthetic follow-through reminder",
		Body:    "Hello {{recipient.name}}. Review {{outcome.url}} at {{review.date}}.",
	}
	zero := int64(0)
	save := outcomes.PlanCommand{
		ID:            uuid.New(),
		Action:        "save",
		SourceVersion: source.Version.Version,
		Schedule:      &schedule,
		Message:       &message,
		Reason:        "Explicit native load follow-through review",
	}
	result.Saved, err = prepareCall[outcomes.PlanView](
		ctx,
		p,
		organiser,
		http.MethodPut,
		reviewPath,
		&zero,
		save,
	)
	if err != nil {
		return result, err
	}
	if err = followupAcknowledgement(
		result.Saved,
		id,
		source,
		organiser,
		save,
		1,
		due,
	); err != nil {
		return result, err
	}
	if result.Saved.State != "draft" || result.Saved.Message != message {
		return result, fmt.Errorf(
			"%w: original review draft changed authored message/state",
			errCohort,
		)
	}
	approve := outcomes.PlanCommand{
		ID: uuid.New(), Action: "approve", SourceVersion: source.Version.Version,
		Reason: "Approve exact current owner, source, native inbox and future wall schedule",
	}
	revision := result.Saved.Version
	result.Approved, err = prepareCall[outcomes.PlanView](
		ctx,
		p,
		organiser,
		http.MethodPut,
		reviewPath,
		&revision,
		approve,
	)
	if err != nil {
		return result, err
	}
	if err = followupAcknowledgement(
		result.Approved,
		id,
		source,
		organiser,
		approve,
		2,
		due,
	); err != nil {
		return result, err
	}
	if result.Approved.State != "approved" || result.Approved.EffectiveState != "approved" ||
		result.Approved.Message != message || len(result.Approved.Blockers) != 0 {
		return result, fmt.Errorf(
			"%w: original native reminder approval is not currently executable",
			errCohort,
		)
	}
	result.Owner, err = p.readFollowupOwner(ctx, owner, reviewPath, result.Approved)
	return result, err
}

func (p *NativePreparer) readFollowupOwner(
	ctx context.Context,
	owner *Session,
	path string,
	approved outcomes.PlanView,
) (outcomes.PlanView, error) {
	view, err := prepareCall[outcomes.PlanView](ctx, p, owner, http.MethodGet, path, nil, nil)
	if err != nil {
		return view, err
	}
	if view.ID != approved.ID || view.OwnerID != owner.Person() ||
		view.Version != approved.Version || view.EffectiveState != "approved" ||
		view.Message != (outcomes.ReviewMessage{}) || view.Reason != "" || view.ActorID != uuid.Nil {
		return view, fmt.Errorf(
			"%w: assigned-owner current reading lost scope or exposed organiser composition",
			errCohort,
		)
	}
	return view, nil
}

func followupAcknowledgement(
	view outcomes.PlanView,
	id uuid.UUID,
	source outcomes.View,
	actor *Session,
	command outcomes.PlanCommand,
	version int64,
	due time.Time,
) error {
	if view.ID != id || view.OutcomeID != source.ID || view.Version != version ||
		view.CommandID != command.ID || view.ActorID != actor.Person() ||
		view.OwnerID != *source.OwnerID || view.SourceVersion != source.Version.Version ||
		!view.DueAt.Equal(due) {
		return fmt.Errorf(
			"%w: original follow-up command changed exact identity/source/owner/timing",
			errCohort,
		)
	}
	return nil
}
