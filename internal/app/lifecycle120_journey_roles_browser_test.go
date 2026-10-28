package app

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

type lifecycleBrowserRoleGrant struct {
	Actor    string    `json:"actor"`
	PersonID uuid.UUID `json:"person_id"`
	Role     string    `json:"role"`
}
type lifecycleBrowserRoleCheck struct {
	EventID   uuid.UUID `json:"event_id"`
	EventName string    `json:"event_name"`
	HelpID    uuid.UUID `json:"help_id"`
	EventRead bool      `json:"event_read"`
	StaffRead bool      `json:"staff_read"`
	HelpRead  bool      `json:"help_read"`
	Present   bool      `json:"present"`
	Logistics bool      `json:"logistics"`
	OwnsEvent bool      `json:"owns_event"`
}
type lifecycleBrowserRoleCase struct {
	Name   string                      `json:"name"`
	Actor  string                      `json:"actor"`
	Checks []lifecycleBrowserRoleCheck `json:"checks"`
}
type lifecycleBrowserRolesPhase struct {
	EventName   string                      `json:"event_name"`
	Grants      []lifecycleBrowserRoleGrant `json:"grants,omitempty"`
	Case        *lifecycleBrowserRoleCase   `json:"case,omitempty"`
	PeerEventID uuid.UUID                   `json:"peer_event_id,omitempty"`
	HelpID      uuid.UUID                   `json:"help_id,omitempty"`
	PeerHelpID  uuid.UUID                   `json:"peer_help_id,omitempty"`
	MentorID    uuid.UUID                   `json:"mentor_id,omitempty"`
	MentorName  string                      `json:"mentor_name,omitempty"`
}
type lifecycleBrowserRoleReceipt struct {
	Stage                             string                     `json:"stage"`
	UIStaffGrants                     int                        `json:"ui_staff_grants"`
	Cases                             []lifecycleBrowserRoleCase `json:"cases"`
	NativeEventChecks                 int                        `json:"native_event_checks"`
	MentorRevocationCleared           bool                       `json:"mentor_revocation_cleared"`
	OtherEventRetained                bool                       `json:"other_event_retained"`
	NativeRevocationAndRestoreAudited bool                       `json:"native_revocation_and_restore_audited"`
}

func lifecycleRoleMember(item *lifecycleJourneyEvent, person, teamID uuid.UUID) bool {
	if slices.Contains(item.attendance.Withdrawn, person) {
		return false
	}
	for _, team := range item.teams.Teams {
		if team.State != "published" || teamID != uuid.Nil && team.ID != teamID {
			continue
		}
		for _, member := range team.Published {
			if member.ID == person {
				return true
			}
		}
	}
	return false
}

func (r *lifecycleJourney) roleBrowserPhase(
	t *testing.T,
	sequence int,
	action string,
	item *lifecycleJourneyEvent,
	roles *lifecycleBrowserRolesPhase,
) lifecycleBrowserAcknowledgement {
	t.Helper()
	var clock struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(t.Context(), http.MethodGet, "/clock", nil, &clock); err != nil {
		t.Fatal(err)
	}
	return r.awaitBrowserPhase(
		t,
		lifecycleBrowserPhase{
			Sequence: sequence,
			Action:   action,
			EventID:  item.view.ID,
			Now:      clock.Now,
			Roles:    roles,
		},
	)
}

func (r *lifecycleJourney) readRoleEvent(t *testing.T, item *lifecycleJourneyEvent) event.View {
	t.Helper()
	var view event.View
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String(),
		-1,
		nil,
		&view,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	return view
}

func (r *lifecycleJourney) verifyRoleGrants(
	t *testing.T,
	item *lifecycleJourneyEvent,
	grants []lifecycleBrowserRoleGrant,
) {
	t.Helper()
	var staff struct {
		Items []struct {
			PersonID uuid.UUID `json:"person_id"`
			Role     string    `json:"role"`
			Active   bool      `json:"active"`
		} `json:"items"`
	}
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		"/api/v1/events/"+item.view.ID.String()+"/staff",
		-1,
		nil,
		&staff,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	for _, grant := range grants {
		found := false
		for _, member := range staff.Items {
			if member.PersonID == grant.PersonID && member.Role == grant.Role && member.Active {
				found = true
			}
		}
		if !found {
			t.Fatal(
				"normal staff UI did not establish the exact current native grant",
				grant.Actor,
				grant.Role,
			)
		}
	}
}

func (r *lifecycleJourney) roleMentorAudit(
	t *testing.T,
	item *lifecycleJourneyEvent,
	mentor uuid.UUID,
) []contract.AuditEvent {
	t.Helper()
	rows := []contract.AuditEvent{}
	cursor := int64(0)
	for {
		var page contract.AuditPage
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			fmt.Sprintf("/api/v1/events/%s/audit?cursor=%d&limit=100", item.view.ID, cursor),
			-1,
			nil,
			&page,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Items {
			if row.TargetId == mentor && row.Metadata.Role != nil &&
				string(*row.Metadata.Role) == "mentor" &&
				(row.Action == "role.granted" || row.Action == "role.revoked") {
				rows = append(rows, row)
			}
		}
		if len(page.Items) == 0 {
			return rows
		}
		if page.NextCursor <= cursor {
			t.Fatal("native audit cursor did not advance")
		}
		cursor = page.NextCursor
	}
}

func (r *lifecycleJourney) verifyRoleCase(
	t *testing.T,
	client *lifecycleClient,
	current lifecycleBrowserRoleCase,
) {
	t.Helper()
	for _, check := range current.Checks {
		base := "/api/v1/events/" + check.EventID.String()
		status := http.StatusForbidden
		if check.EventRead {
			status = http.StatusOK
		}
		if current.Actor == "" {
			status = http.StatusUnauthorized
		}
		var view event.View
		if err := client.call(
			t.Context(),
			http.MethodGet,
			base,
			-1,
			nil,
			&view,
			status,
		); err != nil {
			t.Fatal(current.Name, "event authority", err)
		}
		if status == http.StatusOK &&
			(view.CanReadStaff != check.StaffRead || view.CanEdit != check.StaffRead || view.CanPresent != check.Present || view.CanReadLogistics != check.Logistics || view.CanManageOwner != check.OwnsEvent) {
			t.Fatal("native actor capabilities differ from the expected live role", current.Name)
		}
		status = http.StatusForbidden
		if check.StaffRead {
			status = http.StatusOK
		}
		if current.Actor == "" {
			status = http.StatusUnauthorized
		}
		if err := client.call(
			t.Context(),
			http.MethodGet,
			base+"/staff",
			-1,
			nil,
			nil,
			status,
		); err != nil {
			t.Fatal(current.Name, "private staff authority", err)
		}
		for _, protected := range []struct {
			path    string
			allowed bool
		}{{"/presentation", check.Present}, {"/logistics", check.Logistics}} {
			status = http.StatusForbidden
			if protected.allowed {
				status = http.StatusOK
			}
			if current.Actor == "" {
				status = http.StatusUnauthorized
			}
			if err := client.call(
				t.Context(),
				http.MethodGet,
				base+protected.path,
				-1,
				nil,
				nil,
				status,
			); err != nil {
				t.Fatal(current.Name, protected.path, err)
			}
		}
		status = http.StatusNotFound
		if check.HelpRead {
			status = http.StatusOK
		}
		if current.Actor == "" {
			status = http.StatusUnauthorized
		}
		if err := client.call(
			t.Context(),
			http.MethodGet,
			base+"/help/"+check.HelpID.String(),
			-1,
			nil,
			nil,
			status,
		); err != nil {
			t.Fatal(current.Name, "private help authority", err)
		}
	}
}

func (r *lifecycleJourney) checkLiveRolesInBrowser(t *testing.T) {
	t.Helper()
	if osMode := os.Getenv(
		"HACKBOX_LIFECYCLE_BROWSER_PHASES",
	); osMode != "role-matrix" &&
		osMode != "ui-journey" {
		return
	}
	actors := map[string]*lifecycleClient{
		"owner":    r.owner,
		"mentor-a": r.events[0].cohort.staff["mentor-a"],
		"judge-a":  r.events[0].cohort.staff["judge-a"],
	}
	grants := []lifecycleBrowserRoleGrant{}
	for _, selection := range []struct{ actor, role string }{{"coordinator", "organiser"}, {"mc", "mc"}, {"logistics", "logistics"}} {
		client, err := signInLifecycle(
			t.Context(),
			"http://public.localhost:8080",
			"http://provider.localhost:8080",
			selection.actor,
		)
		if err != nil {
			t.Fatal(err)
		}
		actors[selection.actor] = client
		grants = append(
			grants,
			lifecycleBrowserRoleGrant{
				Actor:    selection.actor,
				PersonID: client.person,
				Role:     selection.role,
			},
		)
	}
	receipt := &lifecycleBrowserRoleReceipt{Stage: "live", Cases: []lifecycleBrowserRoleCase{}}
	for i := range r.events {
		item := &r.events[i]
		before := r.readRoleEvent(t, item)
		ack := r.roleBrowserPhase(
			t,
			4+i,
			"assign-role-staff",
			item,
			&lifecycleBrowserRolesPhase{EventName: item.view.Name, Grants: grants},
		)
		if !ack.Saved {
			t.Fatal("staff phase did not report UI writes")
		}
		r.verifyRoleGrants(t, item, grants)
		after := r.readRoleEvent(t, item)
		if after.Revision != before.Revision+int64(len(grants)) || after.Stage != "live" ||
			after.ConfirmedAttendees != before.ConfirmedAttendees ||
			!reflect.DeepEqual(after.Configuration, before.Configuration) {
			t.Fatal("UI staff grants changed competition configuration, attendance or stage")
		}
		item.view = after
		receipt.UIStaffGrants += len(grants)
	}
	first := &r.events[0]
	var captain, attendee, other *lifecycleClient
	for _, team := range first.teams.Teams {
		if team.State != "published" {
			continue
		}
		if team.Captain != nil && lifecycleRoleMember(first, *team.Captain, team.ID) &&
			(captain == nil || team.ID == first.liveHelp.TeamID) {
			captain = r.participant(t, *team.Captain)
		}
		if team.ID == first.liveHelp.TeamID {
			for _, member := range team.Published {
				if (team.Captain == nil || member.ID != *team.Captain) &&
					lifecycleRoleMember(first, member.ID, team.ID) {
					attendee = r.participant(t, member.ID)
					break
				}
			}
		}
	}
	if captain == nil || attendee == nil {
		t.Fatal("live role matrix lacks distinct current captain and teammate")
	}
	for _, team := range first.teams.Teams {
		if team.ID == first.liveHelp.TeamID || team.State != "published" {
			continue
		}
		for _, member := range team.Published {
			if member.ID != captain.person && lifecycleRoleMember(first, member.ID, team.ID) {
				other = r.participant(t, member.ID)
				break
			}
		}
		if other != nil {
			break
		}
	}
	if other == nil {
		t.Fatal("live role matrix lacks a distinct current other-team attendee")
	}
	actorAlias := func(client *lifecycleClient) string {
		for index, c := range first.cohort.clients {
			if c.person == client.person {
				return fmt.Sprintf("attendee-%02d", index+1)
			}
		}
		t.Fatal("role participant lacks an original cohort alias")
		return ""
	}
	for _, person := range []*lifecycleClient{captain, attendee, other} {
		actors[actorAlias(person)] = person
	}
	for _, alias := range []string{"stranger", "former-organiser"} {
		client, err := signInLifecycle(
			t.Context(),
			"http://public.localhost:8080",
			"http://provider.localhost:8080",
			alias,
		)
		if err != nil {
			t.Fatal(err)
		}
		actors[alias] = client
	}
	actors[""] = &lifecycleClient{
		origin: "http://public.localhost:8080",
		http:   &http.Client{Timeout: 15 * time.Second},
	}
	cases := []struct {
		name, actor                 string
		staff, event, help, present bool
	}{
		{"organiser", "owner", true, true, true, true},
		{"coordinator", "coordinator", true, true, true, true},
		{"captain", actorAlias(captain), false, true, true, false},
		{"attendee", actorAlias(attendee), false, true, true, false},
		{"other-team", actorAlias(other), false, true, false, false},
		{"mentor", "mentor-a", false, true, true, false},
		{"judge", "judge-a", false, true, false, false},
		{"mc", "mc", false, true, false, true},
		{"logistics", "logistics", false, true, false, false},
		{"member-without-event", "stranger", false, false, false, false},
		{"revoked-organiser", "former-organiser", false, false, false, false},
		{"anonymous-stranger", "", false, false, false, false},
	}
	for index, entry := range cases {
		current := lifecycleBrowserRoleCase{
			Name:   entry.name,
			Actor:  entry.actor,
			Checks: []lifecycleBrowserRoleCheck{},
		}
		client := actors[entry.actor]
		for i := range r.events {
			item := &r.events[i]
			eventRead, helpRead := entry.event, entry.help
			if entry.name == "captain" || entry.name == "attendee" || entry.name == "other-team" {
				eventRead = lifecycleRoleMember(item, client.person, uuid.Nil)
				helpRead = lifecycleRoleMember(item, client.person, item.liveHelp.TeamID)
			}
			current.Checks = append(
				current.Checks,
				lifecycleBrowserRoleCheck{
					EventID:   item.view.ID,
					EventName: item.view.Name,
					HelpID:    item.liveHelp.RequestID,
					EventRead: eventRead,
					StaffRead: entry.staff,
					HelpRead:  helpRead,
					Present:   entry.present,
					Logistics: entry.name == "logistics",
					OwnsEvent: entry.name == "organiser",
				},
			)
		}
		r.roleBrowserPhase(
			t,
			6+index,
			"check-live-role",
			first,
			&lifecycleBrowserRolesPhase{EventName: first.view.Name, Case: &current},
		)
		r.verifyRoleCase(t, client, current)
		receipt.NativeEventChecks += len(current.Checks)
		receipt.Cases = append(receipt.Cases, current)
	}
	mentor := first.cohort.staff["mentor-a"]
	before := r.roleMentorAudit(t, first, mentor.person)
	peerBefore := r.roleMentorAudit(t, &r.events[1], mentor.person)
	ack := r.roleBrowserPhase(
		t,
		18,
		"revoke-mentor-event",
		first,
		&lifecycleBrowserRolesPhase{
			EventName:   first.view.Name,
			HelpID:      first.liveHelp.RequestID,
			PeerEventID: r.events[1].view.ID,
			PeerHelpID:  r.events[1].liveHelp.RequestID,
			MentorID:    mentor.person,
			MentorName:  mentor.name,
		},
	)
	if !ack.Saved {
		t.Fatal("mentor revocation/restore did not use UI writes")
	}
	r.verifyRoleGrants(
		t,
		first,
		[]lifecycleBrowserRoleGrant{{Actor: "mentor-a", PersonID: mentor.person, Role: "mentor"}},
	)
	after := r.roleMentorAudit(t, first, mentor.person)
	peerAfter := r.roleMentorAudit(t, &r.events[1], mentor.person)
	if len(after) != len(before)+2 || !reflect.DeepEqual(after[:len(before)], before) ||
		!reflect.DeepEqual(peerBefore, peerAfter) {
		t.Fatal("mentor revocation rewrote prior or other-event role history")
	}
	revoke, restore := after[len(before)], after[len(before)+1]
	if revoke.Action != "role.revoked" || restore.Action != "role.granted" ||
		revoke.Sequence >= restore.Sequence ||
		revoke.ActorId == nil ||
		restore.ActorId == nil ||
		*revoke.ActorId != r.owner.person ||
		*restore.ActorId != r.owner.person {
		t.Fatal("native role history lacks the ordered owner UI revocation and restoration")
	}
	for i := range r.events {
		item := &r.events[i]
		if err := mentor.call(
			t.Context(),
			http.MethodGet,
			"/api/v1/events/"+item.view.ID.String()+"/help/"+item.liveHelp.RequestID.String(),
			-1,
			nil,
			nil,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		item.view = r.readRoleEvent(t, item)
	}
	receipt.MentorRevocationCleared = true
	receipt.OtherEventRetained = true
	receipt.NativeRevocationAndRestoreAudited = true
	r.liveRoles = receipt
}
