package app

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/google/uuid"
)

type lifecycleBrowserTeamChange struct {
	PersonName string        `json:"person_name"`
	TeamName   string        `json:"team_name"`
	Command    teams.Command `json:"command"`
	Revision   int64         `json:"revision"`
	Last       bool          `json:"last"`
}

func (r *lifecycleJourney) browserTeamActions(
	t *testing.T,
	item *lifecycleJourneyEvent,
	index int,
) func(teams.Command) (teams.Page, error) {
	t.Helper()
	if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") != "ui-journey" {
		return nil
	}
	publications := 0
	return func(command teams.Command) (teams.Page, error) {
		path := "/api/v1/events/" + item.view.ID.String()
		var before, after teams.Page
		var eventBefore, eventAfter event.View
		read := func(page *teams.Page, view *event.View) error {
			if err := r.owner.call(t.Context(), http.MethodGet, path+"/teams", -1,
				nil, page, http.StatusOK); err != nil {
				return err
			}
			return r.owner.call(t.Context(), http.MethodGet, path, -1, nil, view, http.StatusOK)
		}
		if err := read(&before, &eventBefore); err != nil {
			return after, err
		}
		command.Reason = "Synthetic lifecycle reviewed allocation"
		if command.Action == "publish" {
			publications++
		}
		input := &lifecycleBrowserTeamChange{Command: command, Revision: before.Revision, Last: publications == 2}
		for _, person := range before.People {
			if person.ID == command.PersonID {
				input.PersonName = person.Name
			}
		}
		for _, team := range before.Teams {
			if team.ID == command.TeamID {
				input.TeamName = team.Name
			}
		}
		if command.Action == "create" {
			input.TeamName = command.Name
		}
		ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{
			Sequence:   301 + index*100 + len(item.browserTeamCommands),
			Action:     "team-change",
			TeamChange: input,
		})
		if !ack.Saved {
			return after, fmt.Errorf("team command lacks its real UI acknowledgement")
		}
		if err := read(&after, &eventAfter); err != nil {
			return after, err
		}
		// Team commands share the native event revision. Only that revision may
		// change in the event projection, apart from derived readiness blockers.
		// Stage, admission and policy stay fixed.
		eventBefore.Revision++
		eventBefore.Blockers = eventAfter.Blockers
		if after.Revision != before.Revision+1 || !reflect.DeepEqual(eventBefore, eventAfter) ||
			!reflect.DeepEqual(before.People, after.People) ||
			!reflect.DeepEqual(before.Requests, after.Requests) || before.Frozen != after.Frozen {
			return after, fmt.Errorf("UI team change altered unrelated native event or admission state")
		}
		if err := verifyLifecycleBrowserTeamChange(before, after, command); err != nil {
			return after, err
		}
		item.browserTeamCommands = append(item.browserTeamCommands, command.Action)
		return after, nil
	}
}

func verifyLifecycleBrowserTeamChange(before, after teams.Page, command teams.Command) error {
	previous := make(map[uuid.UUID]teams.Team, len(before.Teams))
	for _, team := range before.Teams {
		previous[team.ID] = team
	}
	created, assigned := 0, 0
	for _, team := range after.Teams {
		old, exists := previous[team.ID]
		if !exists {
			if command.Action != "create" || team.Name != command.Name ||
				team.Captain == nil || *team.Captain != command.PersonID ||
				len(team.Draft) != 1 || team.Draft[0].ID != command.PersonID || len(team.Published) != 0 {
				return fmt.Errorf("UI create differs from its reviewed native captain or roster")
			}
			created++
			continue
		}
		delete(previous, team.ID)
		if command.Action != "publish" && (!reflect.DeepEqual(old.Published, team.Published) ||
			!reflect.DeepEqual(old.PublishedCaptain, team.PublishedCaptain)) {
			return fmt.Errorf("UI draft change altered published membership")
		}
		switch command.Action {
		case "create":
			if !reflect.DeepEqual(old, team) {
				return fmt.Errorf("UI create altered an existing team")
			}
		case "unlock":
			if team.ID == command.TeamID {
				old.AllocationLocked = false
			}
			if !reflect.DeepEqual(old, team) {
				return fmt.Errorf("UI unlock altered more than the reviewed allocation lock")
			}
		case "move":
			withoutMoved := func(people []teams.Person) []teams.Person {
				var others []teams.Person
				for _, person := range people {
					if person.ID != command.PersonID {
						others = append(others, person)
					}
				}
				return others
			}
			if !reflect.DeepEqual(withoutMoved(old.Draft), withoutMoved(team.Draft)) {
				return fmt.Errorf("UI move altered another competitor's draft placement")
			}
			for _, person := range team.Draft {
				if person.ID == command.PersonID {
					if team.ID != command.TeamID {
						return fmt.Errorf("UI move retained membership in the wrong team")
					}
					assigned++
				}
			}
			wasSource := false
			for _, person := range old.Draft {
				wasSource = wasSource || person.ID == command.PersonID
			}
			if team.ID != command.TeamID && !wasSource && !reflect.DeepEqual(old, team) {
				return fmt.Errorf("UI move altered an unrelated team")
			}
		case "publish":
			if len(team.Draft) == 0 {
				if team.State != "disbanded" || len(team.Published) != 0 {
					return fmt.Errorf("UI publication did not retire the empty team")
				}
			} else if !reflect.DeepEqual(team.Draft, team.Published) ||
				!reflect.DeepEqual(team.Captain, team.PublishedCaptain) {
				return fmt.Errorf("UI publication differs from its reviewed draft")
			}
		default:
			return fmt.Errorf("unsupported native browser team action %q", command.Action)
		}
	}
	if len(previous) != 0 || (command.Action == "create" && created != 1) ||
		(command.Action == "move" && assigned != 1) {
		return fmt.Errorf("UI team command has missing or duplicate native effects")
	}
	return nil
}

func lifecycleBrowserTeamCommandsComplete(commands []string) bool {
	counts := map[string]int{}
	for _, command := range commands {
		counts[command]++
	}
	return reflect.DeepEqual(counts, map[string]int{"create": 7, "move": 30, "unlock": 7, "publish": 2})
}
