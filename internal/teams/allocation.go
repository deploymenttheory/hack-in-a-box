package teams

import (
	"cmp"
	"crypto/sha256"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

// AllocationCandidate contains only deliberately supplied allocation features.
// Operational profiles and arbitrary registration answers are not engine inputs.
type AllocationCandidate struct {
	ID         uuid.UUID   `json:"id"`
	Skills     []string    `json:"skills"`
	Experience int         `json:"experience"` // 0 unknown; 1 starting; 2 some; 3 experienced.
	PreferWith []uuid.UUID `json:"prefer_with"`
}

type AllocationGroup struct {
	ID       uuid.UUID   `json:"id"`
	Members  []uuid.UUID `json:"members"`
	Locked   bool        `json:"locked"`
	Capacity *int        `json:"capacity"`
}

type AllocationLimits struct {
	MaximumSize  *int `json:"maximum_size"`
	MaximumTeams *int `json:"maximum_teams"`
}

type AllocationPreferences struct {
	Target       int `json:"target"`
	PreferredMin int `json:"preferred_min"`
	PreferredMax int `json:"preferred_max"`
	Minimum      int `json:"minimum"`
}

type AllocationIssue struct {
	Code    string      `json:"code"`
	People  []uuid.UUID `json:"people"`
	TeamID  *uuid.UUID  `json:"team_id"`
	Message string      `json:"message"`
}

type ProposedAllocationTeam struct {
	ID       uuid.UUID   `json:"id"`
	Existing bool        `json:"existing"`
	Locked   bool        `json:"locked"`
	Capacity *int        `json:"capacity"`
	Members  []uuid.UUID `json:"members"`
	Reasons  []string    `json:"reasons"`
}

type AllocationSuggestion struct {
	Revision     int64                    `json:"revision"`
	Seed         string                   `json:"seed"`
	Preferences  AllocationPreferences    `json:"preferences"`
	Limits       AllocationLimits         `json:"limits"`
	Teams        []ProposedAllocationTeam `json:"teams"`
	Unresolved   []uuid.UUID              `json:"unresolved"`
	RetiredTeams []uuid.UUID              `json:"retired_teams"`
	Issues       []AllocationIssue        `json:"issues"`
}

type allocationSnapshot struct {
	EventID  uuid.UUID
	Revision int64
	Frozen   bool
	People   []AllocationCandidate
	Groups   []AllocationGroup
}

type allocationBin struct {
	ProposedAllocationTeam
	limit int
}

// allocate is pure: it neither changes memberships nor creates delivery intents.
// Its caller supplies an authorised, revision-bound snapshot under the event lock.
func allocate(
	snapshot allocationSnapshot,
	seed string,
	limits AllocationLimits,
) (AllocationSuggestion, error) {
	out := AllocationSuggestion{
		Revision: snapshot.Revision,
		Seed:     seed,
		Limits:   limits,
		Preferences: AllocationPreferences{
			Target:       5,
			PreferredMin: 4,
			PreferredMax: 6,
			Minimum:      3,
		},
		Teams:        []ProposedAllocationTeam{},
		Unresolved:   []uuid.UUID{},
		RetiredTeams: []uuid.UUID{},
		Issues:       []AllocationIssue{},
	}
	if snapshot.Frozen {
		return out, ErrFrozen
	}
	people, groups, err := allocationInput(snapshot, seed, limits)
	if err != nil {
		return out, err
	}
	capacity, maxTeams := max(3, len(people)), len(people)
	if limits.MaximumSize != nil {
		capacity = *limits.MaximumSize
	}
	if limits.MaximumTeams != nil {
		maxTeams = *limits.MaximumTeams
	}
	bins, reusable, reserved := prepareAllocationGroups(&out, groups, capacity)
	if len(bins) > maxTeams {
		for id := range people {
			out.Unresolved = append(out.Unresolved, id)
		}
		out.Unresolved = uniqueIDs(out.Unresolved)
		out.Issues = append(
			out.Issues,
			AllocationIssue{
				Code:    "locked_team_limit",
				People:  slices.Clone(out.Unresolved),
				Message: "The locked teams exceed the hard team-count limit. Unlock teams or change the limit.",
			},
		)
		return out, nil
	}
	pool := []AllocationCandidate{}
	for id, candidate := range people {
		if !reserved[id] {
			pool = append(pool, candidate)
		}
	}
	// Reuse the largest available capacities first, with stable ties. A depleted
	// unlocked team is retired: its survivors go to another or a newly formed team.
	slices.SortFunc(reusable, func(a, b AllocationGroup) int {
		if c := cmp.Compare(groupLimit(b, capacity), groupLimit(a, capacity)); c != 0 {
			return c
		}
		return compareID(a.ID, b.ID)
	})
	bestK := additionalAllocationTeams(bins, reusable, len(pool), capacity, maxTeams)
	bins = makeAllocationBins(snapshot.EventID, seed, groups, reusable, bins, bestK, capacity)
	placeAllocationPeople(&out, pool, bins, people, seed)
	explainAllocation(&out, groups, bins, people)
	return out, nil
}

func groupLimit(group AllocationGroup, maximum int) int {
	if group.Capacity != nil {
		return min(maximum, *group.Capacity)
	}
	return maximum
}

func compareID(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) }

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	copy := slices.Clone(ids)
	slices.SortFunc(copy, compareID)
	return slices.Compact(copy)
}

func allocatedTogether(groups []ProposedAllocationTeam, a, b uuid.UUID) bool {
	for _, group := range groups {
		if slices.Contains(group.Members, a) && slices.Contains(group.Members, b) {
			return true
		}
	}
	return false
}

func allocationInput(
	snapshot allocationSnapshot,
	seed string,
	limits AllocationLimits,
) (map[uuid.UUID]AllocationCandidate, []AllocationGroup, error) {
	invalid := func() (map[uuid.UUID]AllocationCandidate, []AllocationGroup, error) {
		return nil, nil, access.ErrInvalid
	}
	if snapshot.EventID == uuid.Nil || len(seed) > 200 || len(snapshot.People) > 10000 ||
		len(snapshot.Groups) > 10000 ||
		(limits.MaximumSize != nil && (*limits.MaximumSize < 3 || *limits.MaximumSize > 10000)) ||
		(limits.MaximumTeams != nil && (*limits.MaximumTeams < 1 || *limits.MaximumTeams > 10000)) {
		return invalid()
	}
	people, err := allocationPeople(snapshot.People)
	if err != nil {
		return nil, nil, err
	}
	groups := slices.Clone(snapshot.Groups)
	seen, assigned := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	for i, group := range groups {
		if group.ID == uuid.Nil || seen[group.ID] ||
			(group.Capacity != nil && (*group.Capacity < 3 || *group.Capacity > 10000)) {
			return invalid()
		}
		seen[group.ID] = true
		groups[i].Members = slices.Clone(group.Members)
		slices.SortFunc(groups[i].Members, compareID)
		for _, id := range group.Members {
			if _, ok := people[id]; !ok || assigned[id] {
				return invalid()
			}
			assigned[id] = true
		}
	}
	slices.SortFunc(groups, func(a, b AllocationGroup) int { return compareID(a.ID, b.ID) })
	return people, groups, nil
}

func orderAllocationCandidates(pool []AllocationCandidate, seed string) {
	frequency := map[string]int{}
	for _, candidate := range pool {
		for _, skill := range candidate.Skills {
			frequency[skill]++
		}
	}
	rarity := func(candidate AllocationCandidate) int {
		score := 0
		for _, skill := range candidate.Skills {
			score += 10000 / frequency[skill]
		}
		return score
	}
	slices.SortFunc(pool, func(a, b AllocationCandidate) int {
		if c := cmp.Compare(b.Experience, a.Experience); c != 0 {
			return c
		}
		if c := cmp.Compare(rarity(b), rarity(a)); c != 0 {
			return c
		}
		left, right := sha256.Sum256(
			[]byte(seed+a.ID.String()),
		), sha256.Sum256(
			[]byte(seed+b.ID.String()),
		)
		return strings.Compare(string(left[:]), string(right[:]))
	})
}

func allocationFit(
	candidate AllocationCandidate,
	members []uuid.UUID,
	people map[uuid.UUID]AllocationCandidate,
) int {
	// Marginal squared distance from five dominates soft feature/pairing scores.
	n := len(members)
	score := ((n+1-5)*(n+1-5) - (n-5)*(n-5)) * 10000
	for _, id := range members {
		other := people[id]
		score += candidate.Experience * other.Experience * 100
		for _, skill := range candidate.Skills {
			if slices.Contains(other.Skills, skill) {
				score += 30
			}
		}
		if slices.Contains(candidate.PreferWith, id) ||
			slices.Contains(other.PreferWith, candidate.ID) {
			score -= 2000
		}
	}
	return score
}

func prepareAllocationGroups(
	out *AllocationSuggestion,
	groups []AllocationGroup,
	capacity int,
) ([]allocationBin, []AllocationGroup, map[uuid.UUID]bool) {
	bins, reusable := []allocationBin{}, []AllocationGroup{}
	reserved := map[uuid.UUID]bool{}
	for _, group := range groups {
		if !group.Locked {
			if len(group.Members) >= 3 {
				reusable = append(reusable, group)
			}
			continue
		}
		for _, id := range group.Members {
			reserved[id] = true
		}
		if len(group.Members) < 3 || len(group.Members) > groupLimit(group, capacity) {
			out.Unresolved = append(out.Unresolved, group.Members...)
			out.Issues = append(out.Issues, AllocationIssue{
				Code:   "locked_group_unresolved",
				TeamID: &group.ID,
				People: slices.Clone(
					group.Members,
				),
				Message: "Explicitly unlock this depleted or over-capacity team before rebalancing; existing assignments are unchanged.",
			})
			continue
		}
		bins = append(bins, allocationBin{ProposedAllocationTeam: ProposedAllocationTeam{
			ID:       group.ID,
			Existing: true,
			Locked:   true,
			Capacity: group.Capacity,
			Members:  slices.Clone(group.Members),
			Reasons:  []string{"Locked members remain together."},
		}, limit: groupLimit(group, capacity)})
	}
	return bins, reusable, reserved
}

func additionalAllocationTeams(
	bins []allocationBin,
	reusable []AllocationGroup,
	poolSize, capacity, maxTeams int,
) int {
	oldMembers, spare := 0, 0
	for _, bin := range bins {
		oldMembers += len(bin.Members)
		spare += bin.limit - len(bin.Members)
	}
	// Enumerate feasible team counts, maximising assigned competitors first and
	// then minimising deviation from five. Four-to-six never becomes a hard cap.
	bestK, bestAssigned, bestCost, totalSpace := 0, -1, 0.0, spare
	for k := 0; k <= min(poolSize/3, maxTeams-len(bins)); k++ {
		if k > 0 {
			teamCapacity := capacity
			if k <= len(reusable) {
				teamCapacity = groupLimit(reusable[k-1], capacity)
			}
			totalSpace += teamCapacity
		}
		assigned, count := min(poolSize, totalSpace), len(bins)+k
		if assigned < 3*k || count == 0 {
			continue
		}
		n := float64(oldMembers + assigned)
		cost := n*n/float64(count) + 25*float64(count)
		if assigned > bestAssigned || (assigned == bestAssigned && cost < bestCost) {
			bestK, bestAssigned, bestCost = k, assigned, cost
		}
	}
	return bestK
}

func makeAllocationBins(
	eventID uuid.UUID,
	seed string,
	groups, reusable []AllocationGroup,
	bins []allocationBin,
	bestK, capacity int,
) []allocationBin {
	used := map[uuid.UUID]bool{}
	for _, group := range groups {
		used[group.ID] = true
	}
	for k := range bestK {
		bin := allocationBin{
			ProposedAllocationTeam: ProposedAllocationTeam{
				Members: []uuid.UUID{},
				Reasons: []string{
					"Balanced declared skills and AI-development experience with seeded, stable ties.",
				},
			},
			limit: capacity,
		}
		if k < len(reusable) {
			group := reusable[k]
			bin.ID, bin.Existing, bin.Capacity, bin.limit = group.ID, true, group.Capacity, groupLimit(
				group,
				capacity,
			)
		} else {
			for suffix := k; ; suffix++ {
				bin.ID = uuid.NewSHA1(eventID, []byte("allocation/"+seed+"/"+strconv.Itoa(suffix)))
				if !used[bin.ID] {
					used[bin.ID] = true
					break
				}
			}
		}
		bins = append(bins, bin)
	}
	return bins
}

func placeAllocationPeople(
	out *AllocationSuggestion,
	pool []AllocationCandidate,
	bins []allocationBin,
	people map[uuid.UUID]AllocationCandidate,
	seed string,
) {
	orderAllocationCandidates(pool, seed)
	for _, candidate := range pool {
		chosen, bestScore, fillingMinimum := -1, 0, false
		for _, bin := range bins {
			fillingMinimum = fillingMinimum || len(bin.Members) < 3
		}
		for index, bin := range bins {
			if len(bin.Members) >= bin.limit || (fillingMinimum && len(bin.Members) >= 3) {
				continue
			}
			score := allocationFit(candidate, bin.Members, people)
			if chosen == -1 || score < bestScore {
				chosen, bestScore = index, score
			}
		}
		if chosen == -1 {
			out.Unresolved = append(out.Unresolved, candidate.ID)
		} else {
			bins[chosen].Members = append(bins[chosen].Members, candidate.ID)
		}
	}
}

func explainAllocation(
	out *AllocationSuggestion,
	groups []AllocationGroup,
	bins []allocationBin,
	people map[uuid.UUID]AllocationCandidate,
) {
	selected := map[uuid.UUID]bool{}
	allSkills, highestExperience := []string{}, 0
	for _, candidate := range people {
		allSkills = append(allSkills, candidate.Skills...)
		highestExperience = max(highestExperience, candidate.Experience)
	}
	slices.Sort(allSkills)
	allSkills = slices.Compact(allSkills)
	for _, bin := range bins {
		selected[bin.ID] = true
		slices.SortFunc(bin.Members, compareID)
		bin.Reasons = append(
			bin.Reasons,
			fmt.Sprintf(
				"%d competitors; target 5, preferred 4–6, hard minimum 3.",
				len(bin.Members),
			),
		)
		if len(bin.Members) < 4 || len(bin.Members) > 6 {
			bin.Reasons = append(
				bin.Reasons,
				"Outside the preferred size range to preserve locked groups and hard constraints; review before approval.",
			)
		}
		out.Teams = append(out.Teams, bin.ProposedAllocationTeam)
		present, experience := map[string]bool{}, 0
		for _, id := range bin.Members {
			for _, skill := range people[id].Skills {
				present[skill] = true
			}
			experience = max(experience, people[id].Experience)
		}
		missing := []string{}
		for _, skill := range allSkills {
			if !present[skill] {
				missing = append(missing, skill)
			}
		}
		if len(missing) > 0 {
			out.Issues = append(
				out.Issues,
				AllocationIssue{
					Code:   "skill_coverage",
					TeamID: &bin.ID,
					People: slices.Clone(bin.Members),
					Message: "Soft skill coverage gap: " + strings.Join(
						missing,
						", ",
					) + ". Review declared skills and locked-group constraints.",
				},
			)
		}
		if experience < highestExperience {
			out.Issues = append(
				out.Issues,
				AllocationIssue{
					Code:    "experience_coverage",
					TeamID:  &bin.ID,
					People:  slices.Clone(bin.Members),
					Message: "This team lacks the highest AI-development experience level declared in the pool; review coverage before approval.",
				},
			)
		}
	}
	for _, group := range groups {
		if !group.Locked && !selected[group.ID] {
			out.RetiredTeams = append(out.RetiredTeams, group.ID)
		}
	}
	if len(out.Unresolved) > 0 {
		out.Issues = append(
			out.Issues,
			AllocationIssue{
				Code:    "unassigned",
				People:  uniqueIDs(out.Unresolved),
				Message: "Hard capacity, locked-group or minimum-size constraints leave these people unresolved. Review limits or explicitly unlock groups; no memberships were changed.",
			},
		)
	}
	ids := make([]uuid.UUID, 0, len(people))
	for id := range people {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, compareID)
	seenPairs := map[string]bool{}
	for _, id := range ids {
		candidate := people[id]
		for _, preferred := range candidate.PreferWith {
			pair := uniqueIDs([]uuid.UUID{candidate.ID, preferred})
			key := pair[0].String() + pair[1].String()
			if !seenPairs[key] && !allocatedTogether(out.Teams, candidate.ID, preferred) {
				out.Issues = append(
					out.Issues,
					AllocationIssue{
						Code:    "pairing_preference",
						People:  []uuid.UUID{candidate.ID, preferred},
						Message: "A soft pairing preference could not be met alongside team balance and hard constraints.",
					},
				)
			}
			seenPairs[key] = true
		}
	}
	out.Unresolved = uniqueIDs(out.Unresolved)
	slices.SortFunc(out.RetiredTeams, compareID)
}

func allocationPeople(candidates []AllocationCandidate) (map[uuid.UUID]AllocationCandidate, error) {
	people := map[uuid.UUID]AllocationCandidate{}
	for _, person := range candidates {
		if _, duplicate := people[person.ID]; duplicate || person.ID == uuid.Nil ||
			person.Experience < 0 ||
			person.Experience > 3 ||
			len(person.Skills) > 32 ||
			len(person.PreferWith) > 10 {
			return nil, access.ErrInvalid
		}
		person.Skills = slices.Clone(person.Skills)
		for i, skill := range person.Skills {
			person.Skills[i] = strings.ToLower(strings.TrimSpace(skill))
			if person.Skills[i] == "" || len(person.Skills[i]) > 80 {
				return nil, access.ErrInvalid
			}
		}
		slices.Sort(person.Skills)
		person.Skills = slices.Compact(person.Skills)
		person.PreferWith = uniqueIDs(person.PreferWith)
		people[person.ID] = person
	}
	for _, person := range people {
		for _, id := range person.PreferWith {
			if _, ok := people[id]; !ok || id == person.ID {
				return nil, access.ErrInvalid
			}
		}
	}
	return people, nil
}
