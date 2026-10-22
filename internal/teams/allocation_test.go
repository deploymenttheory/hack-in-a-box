package teams

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

func allocationFixture(count int) allocationSnapshot {
	s := allocationSnapshot{EventID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("synthetic allocation")), Revision: 5, Groups: []AllocationGroup{}}
	for i := range count {
		s.People = append(s.People, AllocationCandidate{ID: uuid.NewSHA1(s.EventID, []byte(strconv.Itoa(i))), Skills: []string{[]string{"development", "design"}[i%2]}, Experience: 1 + i%3, PreferWith: []uuid.UUID{}})
	}
	return s
}

func allocationMembers(s allocationSnapshot, start, end int) []uuid.UUID {
	ids := []uuid.UUID{}
	for _, p := range s.People[start:end] {
		ids = append(ids, p.ID)
	}
	return ids
}

func assertAllocationInvariants(t *testing.T, s allocationSnapshot, limits AllocationLimits, out AllocationSuggestion) {
	t.Helper()
	seen := map[uuid.UUID]bool{}
	if limits.MaximumTeams != nil && len(out.Teams) > *limits.MaximumTeams {
		t.Fatal("violated hard team count")
	}
	for _, group := range out.Teams {
		if len(group.Members) < 3 || (limits.MaximumSize != nil && len(group.Members) > *limits.MaximumSize) || (group.Capacity != nil && len(group.Members) > *group.Capacity) {
			t.Fatalf("invalid proposed size: %+v", group)
		}
		for _, id := range group.Members {
			if seen[id] {
				t.Fatal("competitor appears twice")
			}
			seen[id] = true
		}
	}
	for _, id := range out.Unresolved {
		if seen[id] {
			t.Fatal("unresolved competitor also assigned")
		}
		seen[id] = true
	}
	if len(seen) != len(s.People) {
		t.Fatalf("lost competitors: %d / %d", len(seen), len(s.People))
	}
	for _, p := range s.People {
		if !seen[p.ID] {
			t.Fatal("lost input competitor")
		}
	}
	for _, original := range s.Groups {
		if !original.Locked {
			continue
		}
		for _, person := range original.Members {
			for _, proposed := range out.Teams {
				if slices.Contains(proposed.Members, person) && proposed.ID != original.ID {
					t.Fatal("moved a locked competitor")
				}
			}
		}
	}
}

func TestAllocationDeterministicBalancedAndDoesNotMutateInputs(t *testing.T) {
	s := allocationFixture(20)
	before, _ := json.Marshal(s)
	want, err := allocate(s, "event-seed", AllocationLimits{})
	if err != nil {
		t.Fatal(err)
	}
	assertAllocationInvariants(t, s, AllocationLimits{}, want)
	if len(want.Teams) != 4 || len(want.Unresolved) != 0 {
		t.Fatalf("not balanced: %+v", want)
	}
	for _, group := range want.Teams {
		if len(group.Members) != 5 {
			t.Fatal("missed target five")
		}
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("changed caller's declarations or membership")
	}
	slices.Reverse(s.People)
	got, err := allocate(s, "event-seed", AllocationLimits{})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("input ordering changed suggestion: %v", err)
	}
}

func TestAllocationHardLimitsAndUndersizedRemediation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		count      int
		maxSize    int
		locked     bool
		wantTeams  int
		unresolved int
	}{
		{"balanced", 10, 0, false, 2, 0},
		{"capacity", 10, 3, false, 3, 1},
		{"impossible_minimum", 2, 0, false, 0, 2},
		{"locked_full_destination", 8, 6, true, 1, 2},
		{"unlocked_rebalancing_forms_new_group", 8, 6, false, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := allocationFixture(tc.count)
			limits := AllocationLimits{}
			if tc.maxSize > 0 {
				limits.MaximumSize = &tc.maxSize
			}
			var depleted uuid.UUID
			if tc.count == 8 {
				depleted = uuid.NewSHA1(s.EventID, []byte("depleted"))
				s.Groups = []AllocationGroup{{ID: uuid.NewSHA1(s.EventID, []byte("full")), Locked: tc.locked, Members: allocationMembers(s, 0, 6)}, {ID: depleted, Members: allocationMembers(s, 6, 8)}}
			}
			out, err := allocate(s, "seed", limits)
			if err != nil {
				t.Fatal(err)
			}
			assertAllocationInvariants(t, s, limits, out)
			if len(out.Teams) != tc.wantTeams || len(out.Unresolved) != tc.unresolved {
				t.Fatalf("unexpected outcome: %+v", out)
			}
			if tc.count == 8 && !slices.Contains(out.RetiredTeams, depleted) {
				t.Fatal("depleted group was reused rather than proposing survivor reassignment")
			}
		})
	}
}

func TestAllocationPreferencesAreNotHardLimits(t *testing.T) {
	s := allocationFixture(9)
	s.Groups = []AllocationGroup{{ID: uuid.New(), Members: allocationMembers(s, 0, 7), Locked: true}}
	out, err := allocate(s, "same", AllocationLimits{})
	if err != nil || len(out.Teams) != 1 || len(out.Teams[0].Members) != 9 || len(out.Unresolved) != 0 {
		t.Fatalf("soft maximum became a hard cap: %+v %v", out, err)
	}
	assertAllocationInvariants(t, s, AllocationLimits{}, out)
	cap := 6
	out, err = allocate(s, "same", AllocationLimits{MaximumSize: &cap})
	if err != nil || len(out.Unresolved) != 9 || len(out.Teams) != 0 {
		t.Fatalf("invalid locked group was split or capacity violated: %+v %v", out, err)
	}
}

func TestAllocationExplainsSparseSkillsAndLockedPairingConflict(t *testing.T) {
	s := allocationFixture(10)
	for i := range s.People {
		s.People[i].Skills = []string{"development"}
	}
	s.People[0].Skills = []string{"rare-design"}
	s.People[0].PreferWith = []uuid.UUID{s.People[5].ID}
	s.Groups = []AllocationGroup{{ID: uuid.New(), Members: allocationMembers(s, 0, 5), Locked: true}, {ID: uuid.New(), Members: allocationMembers(s, 5, 10), Locked: true}}
	out, err := allocate(s, "coverage", AllocationLimits{})
	if err != nil {
		t.Fatal(err)
	}
	assertAllocationInvariants(t, s, AllocationLimits{}, out)
	codes := map[string]bool{}
	for _, issue := range out.Issues {
		codes[issue.Code] = true
	}
	if !codes["skill_coverage"] || !codes["pairing_preference"] {
		t.Fatalf("missing soft-constraint explanations: %+v", out.Issues)
	}
	slices.Reverse(s.Groups)
	got, err := allocate(s, "coverage", AllocationLimits{})
	if err != nil || !reflect.DeepEqual(out, got) {
		t.Fatal("group order changed output")
	}
}

func TestAllocationRejectsFrozenAndInvalidSnapshots(t *testing.T) {
	s := allocationFixture(6)
	s.Frozen = true
	if _, err := allocate(s, "seed", AllocationLimits{}); !errors.Is(err, ErrFrozen) {
		t.Fatal("allocated frozen competitors")
	}
	s.Frozen = false
	s.People[1].ID = s.People[0].ID
	if _, err := allocate(s, "seed", AllocationLimits{}); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("accepted duplicate competitors")
	}
}

func TestAllocationSizeAndCapacityInvariants(t *testing.T) {
	for n := 0; n <= 24; n++ {
		for size := 3; size <= 6; size++ {
			for teams := 1; teams <= 4; teams++ {
				s := allocationFixture(n)
				limits := AllocationLimits{MaximumSize: &size, MaximumTeams: &teams}
				out, err := allocate(s, "invariants", limits)
				if err != nil {
					t.Fatalf("n=%d size=%d teams=%d: %v", n, size, teams, err)
				}
				assertAllocationInvariants(t, s, limits, out)
			}
		}
	}
}
