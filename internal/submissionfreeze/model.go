// Package submissionfreeze owns repository evidence for native frozen submissions.
// It never accepts submissions or derives competition outcomes from GitHub activity.
package submissionfreeze

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var shaPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Source is the immutable cutoff binding, including an explicit applicability
// result. RepositoryID is zero only when no managed numeric identity exists.
type Source struct {
	ManagedEvidenceIDs    []uuid.UUID `json:"managed_evidence_ids"`
	UncapturedEvidenceIDs []uuid.UUID `json:"uncaptured_evidence_ids"`
	Audiences             []string    `json:"audiences"`
	ID                    uuid.UUID   `json:"id"`
	EventID               uuid.UUID   `json:"event_id"`
	TeamID                uuid.UUID   `json:"team_id"`
	EntryID               uuid.UUID   `json:"entry_id"`
	SubmissionRevision    *int64      `json:"submission_revision"`
	NativeOutcome         string      `json:"native_outcome"`
	Cutoff                time.Time   `json:"cutoff"`
	RecordedAt            time.Time   `json:"recorded_at"`
	RepositoryID          int64       `json:"repository_id"`
	RepositoryURL         string      `json:"repository_url"`
	Branch                string      `json:"branch"`
	ConfigurationRevision int64       `json:"configuration_revision"`
	RepositoryProvenance  uuid.UUID   `json:"repository_provenance"`
	Status                string      `json:"status"`
}
type Observation struct {
	ID         uuid.UUID `json:"id"`
	SourceID   uuid.UUID `json:"source_id"`
	ObservedAt time.Time `json:"observed_at"`
	SHA        string    `json:"sha"`
	Status     string    `json:"status"`
	Provenance string    `json:"provenance"`
}
type Push struct {
	ID          uuid.UUID  `json:"id"`
	SHA         string     `json:"sha"`
	Ref         string     `json:"ref"`
	ProviderAt  *time.Time `json:"provider_at"`
	ReceivedAt  time.Time  `json:"received_at"`
	PayloadHash string     `json:"payload_hash"`
}
type Decision struct {
	Audiences             []string    `json:"audiences"`
	ManagedEvidenceIDs    []uuid.UUID `json:"managed_evidence_ids"`
	UncapturedEvidenceIDs []uuid.UUID `json:"uncaptured_evidence_ids"`
	SubmissionRevision    int64       `json:"submission_revision"`
	NativeRevision        int64       `json:"native_revision"`
	ID                    uuid.UUID   `json:"id"`
	SourceID              uuid.UUID   `json:"source_id"`
	Revision              int64       `json:"revision"`
	ActorID               uuid.UUID   `json:"actor_id"`
	At                    time.Time   `json:"at"`
	Action                string      `json:"action"`
	SHA                   string      `json:"sha"`
	Reason                string      `json:"reason"`
	ObservationIDs        []uuid.UUID `json:"observation_ids"`
	PushIDs               []uuid.UUID `json:"push_ids"`
	ExternalEvidence      string      `json:"external_evidence"`
	PriorDecisionID       *uuid.UUID  `json:"prior_decision_id"`
}
type Page struct {
	CurrentReferenceStatus    string        `json:"current_reference_status"`
	NativeRevision            int64         `json:"native_revision"`
	NativeOutcome             string        `json:"native_outcome"`
	CurrentSubmissionRevision *int64        `json:"current_submission_revision"`
	BindingCurrent            bool          `json:"binding_current"`
	Revision                  int64         `json:"revision"`
	Branch                    string        `json:"branch"`
	RepositoryID              int64         `json:"repository_id"`
	RepositoryURL             string        `json:"repository_url"`
	Cutoff                    time.Time     `json:"cutoff"`
	CanConfigure              bool          `json:"can_configure"`
	CanReconcile              bool          `json:"can_reconcile"`
	CanAmend                  bool          `json:"can_amend"`
	Status                    string        `json:"status"`
	Source                    *Source       `json:"source"`
	Observations              []Observation `json:"observations"`
	Decisions                 []Decision    `json:"decisions"`
	Current                   *Decision     `json:"current"`
}
type Command struct {
	SubmissionRevision int64       `json:"submission_revision"`
	NativeRevision     int64       `json:"native_revision"`
	RequestID          uuid.UUID   `json:"request_id"`
	Action             string      `json:"action"`
	SourceID           uuid.UUID   `json:"source_id"`
	Branch             string      `json:"branch"`
	SHA                string      `json:"sha"`
	Reason             string      `json:"reason"`
	ObservationIDs     []uuid.UUID `json:"observation_ids"`
	PushIDs            []uuid.UUID `json:"push_ids"`
	ExternalEvidence   string      `json:"external_evidence"`
	PriorDecisionID    *uuid.UUID  `json:"prior_decision_id"`
	ExpectedRevision   int64       `json:"-"`
}
type Receipt struct {
	Branch           string    `json:"branch"`
	ExpectedRevision int64     `json:"expected_revision"`
	RequestID        uuid.UUID `json:"request_id"`
	Fingerprint      string    `json:"fingerprint"`
	Action           string    `json:"action"`
	Revision         int64     `json:"revision"`
	Decision         *Decision `json:"decision"`
}
type Recovery struct {
	Receipt *Receipt `json:"receipt"`
}
type PushPage struct {
	Items      []Push     `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor"`
}

func validBranch(branch string) bool {
	if len(branch) == 0 || len(branch) > 1000 || strings.HasPrefix(branch, "-") ||
		strings.HasPrefix(branch, "/") ||
		strings.HasSuffix(branch, "/") ||
		strings.HasSuffix(branch, ".") ||
		strings.Contains(branch, "..") ||
		strings.Contains(branch, "@{") ||
		branch == "@" {
		return false
	}
	for _, part := range strings.Split(branch, "/") {
		if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	for _, c := range branch {
		if c <= 32 || c == 127 || strings.ContainsRune("~^:?*[\\", c) {
			return false
		}
	}
	return true
}
