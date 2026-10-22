package mentors

import (
	"cmp"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Validate separates non-waivable invalid assignments from recorded coverage
// gaps. Relationship declarations are explicit; names and email domains never
// imply reporting relationships. Missing availability never implies all-day cover.
func Validate(snapshot Snapshot, assignments []Assignment) []Gap {
	result := []Gap{}
	counts := map[uuid.UUID]map[uuid.UUID]bool{}
	slots := map[uuid.UUID]map[string]Assignment{}
	profiles := map[uuid.UUID]Profile{}
	teams := map[uuid.UUID]Team{}
	for _, p := range snapshot.Profiles {
		profiles[p.PersonID] = p
	}
	for _, t := range snapshot.Teams {
		teams[t.ID] = t
	}
	for _, a := range assignments {
		team, exists := teams[a.TeamID]
		mentor, active := profiles[a.PersonID]
		add := func(code, message string) {
			result = append(
				result,
				Gap{
					Code:     code,
					Message:  message,
					TeamID:   &a.TeamID,
					PersonID: &a.PersonID,
					Hard:     true,
				},
			)
		}
		if !exists || !active || !mentor.Active {
			add("inactive_assignment", "Choose an active event mentor and a current team.")
			continue
		}
		if a.Slot != "primary" && a.Slot != "second" {
			add("invalid_slot", "Choose primary mentor or second reviewer.")
			continue
		}
		if slots[a.TeamID] == nil {
			slots[a.TeamID] = map[string]Assignment{}
		}
		if _, duplicate := slots[a.TeamID][a.Slot]; duplicate {
			add("duplicate_slot", "A team has more than one assignment for this slot.")
		}
		slots[a.TeamID][a.Slot] = a
		if !validWindow(a.Window) || a.Window.Start.Before(snapshot.Live.Start) ||
			a.Window.End.After(snapshot.Live.End) ||
			!available(mentor.Availability, a.Window) {
			add(
				"unavailable",
				"The selected mentor must declare availability for the whole selected interval.",
			)
		}
		if conflicted(snapshot.Conflicts, team, a.PersonID) {
			add(
				"declared_conflict",
				"Own-team or declared reporting conflicts block this assignment.",
			)
		}
		if counts[a.PersonID] == nil {
			counts[a.PersonID] = map[uuid.UUID]bool{}
		}
		counts[a.PersonID][a.TeamID] = true
	}
	result = append(result, capacityGaps(snapshot, counts)...)
	for _, t := range snapshot.Teams {
		primary, hasPrimary := slots[t.ID]["primary"]
		second, hasSecond := slots[t.ID]["second"]
		id := t.ID
		if hasPrimary && hasSecond && primary.PersonID == second.PersonID {
			result = append(
				result,
				Gap{
					Code:    "distinct_reviewer",
					TeamID:  &id,
					Hard:    true,
					Message: "The second reviewer must be distinct from the primary mentor.",
				},
			)
		}
		if !t.Published {
			result = append(
				result,
				Gap{
					Code:    "unpublished_team",
					TeamID:  &id,
					Hard:    true,
					Message: "Publish the competitor team before publishing its mentor coverage.",
				},
			)
		}
		if !hasPrimary {
			result = append(
				result,
				Gap{
					Code:    "missing_primary",
					TeamID:  &id,
					Message: "This team has no primary mentor.",
				},
			)
		} else if !covers(primary.Window, snapshot.Live) {
			result = append(
				result,
				Gap{
					Code:    "partial_primary",
					TeamID:  &id,
					Message: "Primary mentor coverage does not span the complete live hack.",
				},
			)
		}
		if !hasSecond {
			result = append(
				result,
				Gap{
					Code:    "missing_second",
					TeamID:  &id,
					Message: "This team has no distinct second reviewer.",
				},
			)
		}
	}
	slices.SortFunc(result, func(a, b Gap) int {
		if c := strings.Compare(idValue(a.TeamID), idValue(b.TeamID)); c != 0 {
			return c
		}
		if c := strings.Compare(idValue(a.PersonID), idValue(b.PersonID)); c != 0 {
			return c
		}
		return strings.Compare(a.Code, b.Code)
	})
	return result
}

func idValue(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func validWindow(
	w Window,
) bool {
	return !w.Start.IsZero() && !w.End.IsZero() && w.Start.Before(w.End)
}
func covers(a, b Window) bool { return !a.Start.After(b.Start) && !a.End.Before(b.End) }
func available(windows []Window, need Window) bool {
	// Adjacent declared intervals jointly cover a slot; a gap between them cannot.
	sorted := slices.Clone(windows)
	slices.SortFunc(sorted, func(a, b Window) int { return a.Start.Compare(b.Start) })
	cursor := need.Start
	for _, w := range sorted {
		if !validWindow(w) || w.End.Before(cursor) {
			continue
		}
		if w.Start.After(cursor) {
			return false
		}
		if w.End.After(cursor) {
			cursor = w.End
		}
		if !cursor.Before(need.End) {
			return true
		}
	}
	return false
}

func conflicted(conflicts []Conflict, team Team, mentor uuid.UUID) bool {
	if slices.Contains(team.Members, mentor) {
		return true
	}
	for _, c := range conflicts {
		if c.MentorID == mentor && slices.Contains(team.Members, c.CompetitorID) {
			return true
		}
	}
	return false
}

// Suggest is deterministic and read-only. It favours full availability, declared
// skill overlap and lower workload. A scarce or partial roster returns gaps;
// it never overloads a mentor or overrides a known conflict to fill a slot.
func Suggest(snapshot Snapshot) Proposal {
	out := Proposal{Assignments: []Assignment{}}
	teams := slices.Clone(snapshot.Teams)
	slices.SortFunc(
		teams,
		func(a, b Team) int { return strings.Compare(a.ID.String(), b.ID.String()) },
	)
	profiles := slices.Clone(snapshot.Profiles)
	used := map[uuid.UUID]int{}
	primary := map[uuid.UUID]uuid.UUID{}
	for _, slot := range []string{"primary", "second"} {
		for _, team := range teams {
			slices.SortFunc(profiles, func(a, b Profile) int {
				aw, bw := bestWindow(
					a.Availability,
					snapshot.Live,
				), bestWindow(
					b.Availability,
					snapshot.Live,
				)
				if c := cmp.Compare(bw.End.Sub(bw.Start), aw.End.Sub(aw.Start)); c != 0 {
					return c
				}
				if c := cmp.Compare(
					skillOverlap(b.Skills, team.Skills),
					skillOverlap(a.Skills, team.Skills),
				); c != 0 {
					return c
				}
				if c := cmp.Compare(used[a.PersonID], used[b.PersonID]); c != 0 {
					return c
				}
				return strings.Compare(a.PersonID.String(), b.PersonID.String())
			})
			for _, mentor := range profiles {
				cap := snapshot.DefaultCapacity
				if mentor.MaximumTeams != nil {
					cap = *mentor.MaximumTeams
				}
				window := bestWindow(mentor.Availability, snapshot.Live)
				if !mentor.Active || mentor.PersonID == primary[team.ID] ||
					used[mentor.PersonID] >= cap ||
					conflicted(snapshot.Conflicts, team, mentor.PersonID) ||
					!validWindow(window) {
					continue
				}
				out.Assignments = append(
					out.Assignments,
					Assignment{
						TeamID:   team.ID,
						PersonID: mentor.PersonID,
						Slot:     slot,
						Window:   window,
					},
				)
				used[mentor.PersonID]++
				if slot == "primary" {
					primary[team.ID] = mentor.PersonID
				}
				break
			}
		}
	}
	out.Gaps = Validate(snapshot, out.Assignments)
	return out
}

func bestWindow(windows []Window, live Window) Window {
	sorted := slices.Clone(windows)
	slices.SortFunc(sorted, func(a, b Window) int { return a.Start.Compare(b.Start) })
	merged := []Window{}
	for _, w := range sorted {
		if !validWindow(w) {
			continue
		}
		w.Start = w.Start.UTC()
		w.End = w.End.UTC()
		if n := len(merged); n > 0 && !w.Start.After(merged[n-1].End) {
			if w.End.After(merged[n-1].End) {
				merged[n-1].End = w.End
			}
		} else {
			merged = append(merged, w)
		}
	}
	best := Window{}
	for _, w := range merged {
		if w.Start.Before(live.Start) {
			w.Start = live.Start
		}
		if w.End.After(live.End) {
			w.End = live.End
		}
		if validWindow(w) && w.End.Sub(w.Start) > best.End.Sub(best.Start) {
			best = w
		}
	}
	return best
}

func skillOverlap(a, b []string) int {
	set := map[string]bool{}
	for _, s := range a {
		set[strings.ToLower(strings.TrimSpace(s))] = true
	}
	found := map[string]bool{}
	for _, s := range b {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && set[s] {
			found[s] = true
		}
	}
	return len(found)
}

func capacityGaps(snapshot Snapshot, counts map[uuid.UUID]map[uuid.UUID]bool) []Gap {
	result := []Gap{}
	for _, p := range snapshot.Profiles {
		capacity := snapshot.DefaultCapacity
		if p.MaximumTeams != nil {
			capacity = *p.MaximumTeams
		}
		if capacity < 1 || len(counts[p.PersonID]) > capacity {
			id := p.PersonID
			result = append(
				result,
				Gap{
					Code:     "mentor_capacity",
					PersonID: &id,
					Hard:     true,
					Message:  "The mentor workload exceeds the configured maximum assigned teams.",
				},
			)
		}
	}

	return result
}
