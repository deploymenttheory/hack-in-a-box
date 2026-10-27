package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

type lifecycleAdmissions struct {
	clients       []*lifecycleClient
	registrations []registration.View
	staff         map[string]*lifecycleClient
	attendeeForm  registration.FormView
	census        map[string]int64
	offerID       uuid.UUID
	waitlistedID  uuid.UUID
	reserveID     uuid.UUID
	reservePerson uuid.UUID
}

// Every mutation here travels through the authenticated native HTTP owner.
// The final integrated scenario will reuse this phase, including its negative
// gates; independently running this checkpoint does not complete #120.
func admitLifecycleCohort(
	ctx context.Context,
	owner *lifecycleClient,
	current event.View,
) (lifecycleAdmissions, error) {
	return admitLifecycleCohortChecking(ctx, owner, current, 36, nil)
}

type lifecycleAdmissionActions struct {
	Publish func(string) (registration.FormView, error)
	Open    func() (event.View, error)
	Apply   func(string, *lifecycleClient, registration.FormView) (registration.View, error)
	Select  func(string, registration.View) (registration.View, error)
	Go      func() (event.View, error)
	Respond func(string, *lifecycleClient, registration.View, string) (registration.View, error)
}

func admitLifecycleCohortChecking(
	ctx context.Context,
	owner *lifecycleClient,
	current event.View,
	applicants int,
	beforeApplications func(registration.FormView),
) (lifecycleAdmissions, error) {
	return admitLifecycleCohortWithActions(ctx, owner, current, applicants, beforeApplications, nil)
}

func admitLifecycleCohortWithActions(
	ctx context.Context,
	owner *lifecycleClient,
	current event.View,
	applicants int,
	beforeApplications func(registration.FormView),
	actions *lifecycleAdmissionActions,
) (lifecycleAdmissions, error) {
	if applicants != 36 && applicants != 37 {
		return lifecycleAdmissions{}, fmt.Errorf("unsupported synthetic applicant census")
	}
	cohort := lifecycleAdmissions{census: map[string]int64{}, staff: map[string]*lifecycleClient{}}
	base := "/api/v1/events/" + current.ID.String()
	publish := func(audience string) (registration.FormView, error) {
		return publishLifecycleForm(ctx, owner, current.ID, audience)
	}
	if actions != nil && actions.Publish != nil {
		publish = actions.Publish
	}
	form, err := publish("attendee")
	if err != nil {
		return cohort, err
	}
	cohort.attendeeForm = form
	if actions != nil && actions.Open != nil {
		current, err = actions.Open()
	} else {
		current, err = lifecycleTransition(ctx, owner, current.ID, "registration", http.StatusOK)
	}
	if err != nil {
		return cohort, err
	}
	var offerPolicy registration.OfferSettings
	if err := owner.call(
		ctx,
		http.MethodPut,
		base+"/registrations/offer-policy",
		0,
		map[string]any{
			"automated":    true,
			"working_days": 2,
		},
		&offerPolicy,
		http.StatusOK,
	); err != nil {
		return cohort, err
	}
	if !offerPolicy.Automated || offerPolicy.WorkingDays != 2 {
		return cohort, fmt.Errorf("native FIFO automation was not explicitly approved")
	}
	if beforeApplications != nil {
		beforeApplications(form)
	}
	// Selected facilitators use the application/review workflow and cannot
	// inflate the count that determines the competition go/no-go decision.
	for _, role := range []string{"mentor", "judge"} {
		staffForm, err := publish(role)
		if err != nil {
			return cohort, err
		}
		actors := []string{role + "-a", role + "-b"}
		if role == "judge" {
			actors = append(actors, "judge-c")
		}
		for _, actor := range actors {
			staff, err := signInLifecycle(
				ctx,
				"http://public.localhost:8080",
				"http://provider.localhost:8080",
				actor,
			)
			if err != nil {
				return cohort, err
			}
			var application registration.View
			if actions != nil && actions.Apply != nil {
				application, err = actions.Apply(actor, staff, staffForm)
			} else {
				application, err = applyLifecycleForm(ctx, staff, current.ID, staffForm)
			}
			if err != nil {
				return cohort, err
			}
			if application.Kind != "facilitator" || application.State != "received" {
				return cohort, fmt.Errorf("native facilitator application was not received")
			}
			var selected registration.View
			if actions != nil && actions.Select != nil {
				selected, err = actions.Select(actor, application)
			} else {
				err = owner.call(
					ctx,
					http.MethodPost,
					base+"/registrations/"+application.ID.String()+"/decision",
					application.Revision,
					registration.ReviewInput{
						Action:      "selected",
						Reason:      "Synthetic lifecycle role selection",
						Acknowledge: true,
					},
					&selected,
					http.StatusOK,
				)
			}
			if err != nil {
				return cohort, err
			}
			if selected.State != "selected" || selected.PersonID != staff.person {
				return cohort, fmt.Errorf("native facilitator selection did not commit its role")
			}
			cohort.staff[actor] = staff
		}
	}
	for i := 1; i <= applicants; i++ {
		client, err := signInLifecycle(
			ctx,
			"http://public.localhost:8080",
			"http://provider.localhost:8080",
			fmt.Sprintf("attendee-%02d", i),
		)
		if err != nil {
			return cohort, err
		}
		var applied registration.View
		if actions != nil && actions.Apply != nil {
			applied, err = actions.Apply(fmt.Sprintf("attendee-%02d", i), client, form)
		} else {
			applied, err = applyLifecycleForm(ctx, client, current.ID, form)
		}
		if err != nil {
			return cohort, err
		}
		want := "confirmed"
		if i > current.Configuration.Capacity {
			want = "waitlisted"
		}
		if applied.PersonID != client.person || applied.State != want ||
			applied.Kind != "attendee" {
			return cohort, fmt.Errorf("native admission %d did not enforce confirmed capacity", i)
		}
		cohort.clients = append(cohort.clients, client)
		cohort.registrations = append(cohort.registrations, applied)
		if applied.State == "waitlisted" {
			position := int64(i - current.Configuration.Capacity)
			if applied.WaitlistPosition == nil || *applied.WaitlistPosition != position {
				return cohort, fmt.Errorf(
					"overflow competitor did not receive the actual FIFO position",
				)
			}
			if position == 1 {
				cohort.waitlistedID = applied.ID
				cohort.census["overflow_waitlist_position"] = position
			} else if position == 2 {
				cohort.reserveID, cohort.reservePerson = applied.ID, applied.PersonID
				cohort.census["reserve_waitlist_position"] = position
			}
		}
		if i == 29 {
			blocked, err := lifecycleTransition(
				ctx,
				owner,
				current.ID,
				"preparation",
				http.StatusConflict,
			)
			if err != nil {
				return cohort, err
			}
			if blocked.ConfirmedAttendees != 29 {
				return cohort, fmt.Errorf(
					"minimum rejection did not observe exactly 29 confirmed competitors",
				)
			}
			cohort.census["minimum_blocked_confirmed"] = blocked.ConfirmedAttendees
		}
		if i == 30 {
			if err := owner.call(
				ctx,
				http.MethodGet,
				base,
				-1,
				nil,
				&current,
				http.StatusOK,
			); err != nil {
				return cohort, err
			}
			if current.Stage != "registration" || current.ConfirmedAttendees != 30 {
				return cohort, fmt.Errorf(
					"staff counted as competitors or threshold automatically advanced the event",
				)
			}
			cohort.census["minimum_reached_without_auto_advance"] = current.ConfirmedAttendees
		}
	}
	if actions != nil && actions.Go != nil {
		current, err = actions.Go()
	} else {
		current, err = lifecycleTransition(ctx, owner, current.ID, "preparation", http.StatusOK)
	}
	if err != nil {
		return cohort, err
	}
	if current.ConfirmedAttendees != 35 {
		return cohort, fmt.Errorf("explicit competition go decision changed the confirmed count")
	}
	cohort.census["explicit_go_confirmed"] = current.ConfirmedAttendees
	// A withdrawal after go must preserve the stage and cause a real FIFO
	// offer. Accept through its owner before admission closes, during preparation.
	departed := cohort.registrations[34]
	var withdrawn registration.View
	if actions != nil && actions.Respond != nil {
		withdrawn, err = actions.Respond("attendee-35", cohort.clients[34], departed, "withdraw")
	} else {
		err = cohort.clients[34].call(
			ctx,
			http.MethodPost,
			base+"/registrations/"+departed.ID.String()+"/respond",
			departed.Revision,
			map[string]string{"action": "withdraw"},
			&withdrawn,
			http.StatusOK,
		)
	}
	if err != nil {
		return cohort, err
	}
	cohort.registrations[34] = withdrawn
	var own struct {
		Items []registration.View `json:"items"`
	}
	if err := cohort.clients[35].call(
		ctx,
		http.MethodGet,
		base+"/registrations/mine",
		-1,
		nil,
		&own,
		http.StatusOK,
	); err != nil {
		return cohort, err
	}
	if len(own.Items) != 1 || own.Items[0].State != "offered" || own.Items[0].Offer == nil ||
		own.Items[0].WaitlistPosition != nil {
		return cohort, fmt.Errorf("native preparation-phase FIFO offer was not reserved")
	}
	if own.Items[0].ID != cohort.waitlistedID {
		return cohort, fmt.Errorf("FIFO offer went to a different registration")
	}
	cohort.offerID = own.Items[0].Offer.ID
	if err := owner.call(ctx, http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
		return cohort, err
	}
	if current.Stage != "preparation" || current.ConfirmedAttendees != 34 {
		return cohort, fmt.Errorf(
			"an outstanding offer counted as confirmed or withdrawal regressed the stage",
		)
	}
	cohort.census["pending_offer_excluded_confirmed"] = current.ConfirmedAttendees
	var accepted registration.View
	if actions != nil && actions.Respond != nil {
		accepted, err = actions.Respond("attendee-36", cohort.clients[35], own.Items[0], "accept")
	} else {
		err = cohort.clients[35].call(
			ctx,
			http.MethodPost,
			base+"/registrations/"+own.Items[0].ID.String()+"/respond",
			own.Items[0].Revision,
			map[string]string{"action": "accept"},
			&accepted,
			http.StatusOK,
		)
	}
	if err != nil {
		return cohort, err
	}
	cohort.registrations[35] = accepted
	if accepted.Offer == nil || accepted.Offer.ID != cohort.offerID ||
		accepted.Offer.State != "accepted" {
		return cohort, fmt.Errorf("FIFO acceptance did not preserve the original native offer")
	}
	if err := owner.call(ctx, http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
		return cohort, err
	}
	if withdrawn.State != "withdrawn" || accepted.State != "confirmed" ||
		current.Stage != "preparation" ||
		current.ConfirmedAttendees != 35 {
		return cohort, fmt.Errorf(
			"withdrawal/FIFO acceptance changed the go decision or competition census",
		)
	}
	cohort.census["accepted_offer_confirmed"] = current.ConfirmedAttendees
	return cohort, nil
}

func publishLifecycleForm(
	ctx context.Context,
	owner *lifecycleClient,
	eventID uuid.UUID,
	audience string,
) (registration.FormView, error) {
	fieldID := uuid.New()
	definition := forms.Definition{
		ID:       uuid.New(),
		Name:     "Synthetic " + audience + " application",
		Audience: audience,
		Fields: []forms.Field{
			{
				ID:       fieldID,
				Label:    "Synthetic motivation",
				Type:     "text",
				Required: true,
				Choices:  []string{},
			},
		},
	}
	path := "/api/v1/events/" + eventID.String() + "/registration/forms/" + definition.ID.String()
	var saved registration.FormView
	if err := owner.call(
		ctx,
		http.MethodPut,
		path,
		0,
		definition,
		&saved,
		http.StatusOK,
	); err != nil {
		return saved, err
	}
	var published registration.FormView
	err := owner.call(
		ctx,
		http.MethodPost,
		path+"/publish",
		saved.Revision,
		map[string]any{
			"version":                       saved.Version,
			"acknowledge_response_versions": true,
		},
		&published,
		http.StatusOK,
	)
	if err == nil &&
		(published.PublishedVersion == nil || *published.PublishedVersion != saved.Version) {
		err = fmt.Errorf("native form publication did not preserve its exact version")
	}
	return published, err
}

func applyLifecycleForm(
	ctx context.Context,
	actor *lifecycleClient,
	eventID uuid.UUID,
	form registration.FormView,
) (registration.View, error) {
	answers := forms.Answers{}
	for _, field := range form.Definition.Fields {
		answers[field.ID.String()] = json.RawMessage(`"Synthetic lifecycle motivation"`)
	}
	var applied registration.View
	err := actor.call(ctx, http.MethodPost, "/api/v1/events/"+eventID.String()+"/registrations", -1,
		registration.Application{
			ID: uuid.New(), FormID: form.Definition.ID, FormVersion: *form.PublishedVersion,
			Answers: answers, TeamPreference: "solo",
		}, &applied, http.StatusOK)
	return applied, err
}

func lifecycleTransition(
	ctx context.Context,
	owner *lifecycleClient,
	eventID uuid.UUID,
	stage string,
	want int,
) (event.View, error) {
	base := "/api/v1/events/" + eventID.String()
	var current event.View
	if err := owner.call(ctx, http.MethodGet, base, -1, nil, &current, http.StatusOK); err != nil {
		return current, err
	}
	var next event.View
	if want == http.StatusConflict && stage == "preparation" {
		var rejected struct {
			Error struct {
				Code     string        `json:"code"`
				Blockers []event.Issue `json:"blockers"`
			} `json:"error"`
		}
		if err := owner.call(
			ctx,
			http.MethodPost,
			base+"/transition",
			current.Revision,
			map[string]any{
				"stage":                stage,
				"reason":               "Synthetic lifecycle decision",
				"attendance_exception": false,
			},
			&rejected,
			want,
		); err != nil {
			return next, err
		}
		minimumBlocked := false
		for _, blocker := range rejected.Error.Blockers {
			minimumBlocked = minimumBlocked || blocker.Code == "attendance_shortfall"
		}
		if rejected.Error.Code != "event_blocked" || !minimumBlocked {
			return next, fmt.Errorf(
				"preparation was not blocked by the actual confirmed-attendance minimum",
			)
		}
		if err := owner.call(ctx, http.MethodGet, base, -1, nil, &next, http.StatusOK); err != nil {
			return next, err
		}
		if next.Stage != current.Stage || next.Revision != current.Revision {
			return next, fmt.Errorf("blocked preparation changed native event state")
		}
		return next, nil
	}
	err := owner.call(
		ctx,
		http.MethodPost,
		base+"/transition",
		current.Revision,
		map[string]any{
			"stage":                stage,
			"reason":               "Synthetic lifecycle decision",
			"attendance_exception": false,
		},
		&next,
		want,
	)
	return next, err
}

func TestLifecycleComposeAdmission(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Skip("run only in the isolated lifecycle Compose project")
	}
	owner, err := signInLifecycle(
		t.Context(),
		"http://admin.localhost:8080",
		"http://provider.localhost:8080",
		"owner",
	)
	if err != nil {
		t.Fatal(err)
	}
	control, err := newLifecycleControl(os.Getenv("HACKBOX_LIFECYCLE_CONTROL"))
	if err != nil {
		t.Fatal(err)
	}
	defer control.close()
	timeline, err := control.initialTimeline(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var current event.View
	if err := owner.call(
		t.Context(),
		http.MethodPost,
		"/api/v1/events",
		-1,
		lifecycleEventInput("Admission", timeline),
		&current,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	cohort, err := admitLifecycleCohort(t.Context(), owner, current)
	if err != nil {
		t.Fatal(err)
	}
	page, err := allocateLifecycleTeams(t.Context(), owner, current.ID, &cohort, control, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+current.ID.String(),
		-1,
		nil,
		&current,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if current.ConfirmedAttendees != 32 || len(cohort.clients) != 36 || !page.Frozen {
		t.Fatal("final native competition census did not match the exercised cohort")
	}
	cohort.census["after_pair_reassignment_confirmed"] = current.ConfirmedAttendees
	type teamReceipt struct {
		ID      uuid.UUID   `json:"id"`
		State   string      `json:"state"`
		Frozen  bool        `json:"frozen"`
		Members []uuid.UUID `json:"members"`
	}
	var groups []teamReceipt
	for _, team := range page.Teams {
		group := teamReceipt{
			ID:      team.ID,
			State:   team.State,
			Frozen:  team.Frozen,
			Members: []uuid.UUID{},
		}
		for _, member := range team.Published {
			group.Members = append(group.Members, member.ID)
		}
		groups = append(groups, group)
	}
	receipt, err := json.MarshalIndent(map[string]any{
		"checkpoint":                 "native-compose-admission-teams-only",
		"complete_lifecycle":         false,
		"event_id":                   current.ID,
		"applications":               len(cohort.clients),
		"census":                     cohort.census,
		"waitlisted_registration_id": cohort.waitlistedID,
		"accepted_offer_id":          cohort.offerID,
		"frozen_revision":            page.Revision,
		"teams":                      groups,
		"assertions": []string{
			"normal-oidc-for-every-person",
			"published-exact-form-version",
			"native-facilitator-selection",
			"staff-excluded-from-minimum",
			"409-attendance_shortfall-without-state-change",
			"threshold-does-not-auto-advance",
			"explicit-go",
			"capacity-and-fifo",
			"offer-excluded-until-accepted",
			"offer-survives-preparation",
			"target-five-published-teams",
			"actual-withdrawal-leaves-two",
			"409-teams_unready-without-state-change",
			"explicit-unlock-and-reassignment",
			"draft-does-not-change-publication",
			"reviewed-republication",
			"native-freeze",
			"409-teams_frozen-without-state-change",
		},
		"queued_notifications_exercised": false,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join("/artifacts", "admission-teams-only.json"),
		receipt,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Log(
		"partial native HTTP admission/allocation checkpoint; L01-L18 integrated lifecycle remains required",
	)
}
