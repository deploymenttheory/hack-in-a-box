package loadprofile

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
)

// This deliberately recognises ONE failed read-only prefix, not arbitrary
// checkpoint recovery. No domain write, uncertain control, or later phase may
// be replayed merely because a current source happens to look similar.
func parseKnownArchivePrefix(
	raw []byte,
	plan FactoryPlan,
	source SourcePreparation,
	sourceSHA string,
	clock *DockerSetupClock,
) (ReleasePreparation, bool, error) {
	if len(raw) == 0 {
		return ReleasePreparation{}, false, nil
	}
	var previous ReleasePreparation
	if len(raw) > 8<<20 {
		return ReleasePreparation{}, false, fmt.Errorf(
			"%w: bounded known release prefix required",
			errCohort,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&previous) != nil || decoder.Decode(new(any)) != io.EOF {
		return ReleasePreparation{}, false, fmt.Errorf(
			"%w: one exact known release prefix required",
			errCohort,
		)
	}
	if clock == nil || len(clock.Steps) != 0 || !knownReadOnlyArchiveStage(previous) ||
		previous.OriginalSourcesSHA256 != sourceSHA ||
		previous.OriginalAdmissionSHA256 != source.OriginalAdmissionSHA256 {
		return ReleasePreparation{}, false, fmt.Errorf(
			"%w: unrecognised partial native release checkpoint",
			errCohort,
		)
	}
	step := previous.ClockSteps[0]
	publication := plan.Events[4].Input.Configuration.LiveStart.AddDate(0, 0, -7)
	digest, err := hex.DecodeString(step.OutputSHA256)
	if !step.OriginalComplete || !step.Expected.Equal(plan.SetupStart) ||
		!step.Next.Equal(publication) ||
		step.Container != fmt.Sprintf("%s-clock-%d", clock.Project, publication.UnixNano()) ||
		step.Elapsed <= 0 ||
		err != nil ||
		len(digest) != 32 {
		return ReleasePreparation{}, false, fmt.Errorf(
			"%w: prior clock response is unknown or targets a different phase",
			errCohort,
		)
	}
	final := previous.AfterCleanup
	if previous.Authentication.Authenticated != 1000 ||
		previous.Authentication.DistinctPeople != 1000 ||
		previous.Authentication.Failed != 0 ||
		previous.SessionCleanup.OriginalACKs != 1000 ||
		previous.SessionCleanup.Unresolved != 0 ||
		final == nil ||
		final.Events != 5 ||
		final.People != 1000 ||
		final.ActivePeople != 1000 ||
		final.UnexpiredSessions != 0 {
		return ReleasePreparation{}, false, fmt.Errorf(
			"%w: prior setup authentication/cleanup prefix incomplete",
			errCohort,
		)
	}
	if err := knownArchiveReadPrefix(previous.Calls, plan, source); err != nil {
		return ReleasePreparation{}, false, err
	}
	return previous, true, nil
}

func knownArchiveReadPrefix(
	calls []PreparationCall,
	plan FactoryPlan,
	source SourcePreparation,
) error {
	expected := map[string]uuid.UUID{}
	var blockedPath string
	for _, item := range source.Sources {
		if item.Event.OwnerID == nil {
			return fmt.Errorf("%w: original owner binding missing", errCohort)
		}
		path := "/api/v1/events/" + item.Event.ID.String()
		expected[path] = *item.Event.OwnerID
		if item.Event.ID == plan.Events[4].Input.ID {
			blockedPath = path + "/content/" + item.Challenge.ID.String()
		}
	}
	seen := map[string]bool{}
	loggedOut := map[uuid.UUID]bool{}
	blocked := false
	for _, call := range calls {
		if !call.OriginalComplete {
			return fmt.Errorf("%w: uncertain original request cannot resume", errCohort)
		}
		switch {
		case call.Method == http.MethodPost && call.Path == "/api/v1/logout" && call.Status == http.StatusNoContent:
			if call.Actor == uuid.Nil || loggedOut[call.Actor] {
				return fmt.Errorf("%w: duplicate/missing original logout actor", errCohort)
			}
			loggedOut[call.Actor] = true
		case call.Method == http.MethodGet && call.Status == http.StatusOK && expected[call.Path] == call.Actor && !seen[call.Path]:
			seen[call.Path] = true
		case call.Method == http.MethodGet && call.Path == blockedPath && call.Status == http.StatusNotFound && !blocked && call.Actor == expected["/api/v1/events/"+plan.Events[4].Input.ID.String()]:
			blocked = true
		default:
			return fmt.Errorf(
				"%w: partial checkpoint contains an unrecognised request or domain write",
				errCohort,
			)
		}
	}
	if len(seen) != 5 || len(loggedOut) != 1000 || !blocked {
		return fmt.Errorf("%w: known read-only prefix is incomplete", errCohort)
	}
	return nil
}

func knownReadOnlyArchiveStage(previous ReleasePreparation) bool {
	return previous.Step == "archive-native-T-7-release" && !previous.FullLoad &&
		!previous.NativeAdmission &&
		len(previous.ClockSteps) == 1 &&
		len(previous.Formal) == 0 &&
		len(previous.Cutoffs) == 0 &&
		len(previous.CaptureJobs) == 0 &&
		len(previous.Projects) == 0 &&
		len(previous.NativeEvents) == 0 &&
		len(previous.Released) == 0
}
