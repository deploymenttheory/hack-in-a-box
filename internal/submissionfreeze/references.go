package submissionfreeze

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// managedReference only recognises GitHub repository identity. A /commit path
// can identify its repository but supplies NO cutoff/head proof or chosen SHA.
// No URL is fetched, followed or used as a provider request.
func managedReference(raw, canonical string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Scheme != "https" ||
		!strings.EqualFold(u.Host, "github.com") ||
		u.RawQuery != "" {
		return false
	}
	want, err := url.Parse(canonical)
	if err != nil || want.Path == "" {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")
	expected := strings.Split(strings.TrimPrefix(want.Path, "/"), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\\r\n\x00") {
			return false
		}
	}
	if len(expected) != 2 || len(parts) < 2 || !strings.EqualFold(parts[0], expected[0]) ||
		!strings.EqualFold(strings.TrimSuffix(parts[1], ".git"), expected[1]) {
		return false
	}
	if len(parts) == 2 {
		return true
	}
	if len(parts) >= 4 && parts[2] == "tree" {
		return true
	}
	return len(parts) == 4 && parts[2] == "commit" && shaPattern.MatchString(parts[3])
}

func referenceFacts(
	document submissions.Document,
	canonical string,
) (managed, uncaptured []uuid.UUID, audiences []string) {
	managed = []uuid.UUID{}
	uncaptured = []uuid.UUID{}
	audiences = []string{}
	definitions := map[uuid.UUID]submissions.Definition{}
	for _, d := range document.Definitions {
		definitions[d.ID] = d
	}
	for _, e := range document.Evidence {
		if e.Kind != "repository" {
			continue
		}
		if managedReference(e.URL, canonical) {
			managed = append(managed, e.DeliverableID)
		} else {
			uncaptured = append(uncaptured, e.DeliverableID)
			continue
		}
		roles := definitions[e.DeliverableID].Audience
		if len(roles) == 0 {
			roles = []string{"organiser", "mentor", "judge"}
		}
		for _, role := range roles {
			if !slices.Contains(audiences, role) {
				audiences = append(audiences, role)
			}
		}
	}
	return managed, uncaptured, audiences
}

func sourceApplicability(ctx context.Context, q *dbgen.Queries, src *Source) error {
	src.ManagedEvidenceIDs = []uuid.UUID{}
	src.UncapturedEvidenceIDs = []uuid.UUID{}
	src.Audiences = []string{}
	switch src.NativeOutcome {
	case "not_submitted", "withdrawn":
		src.Status = src.NativeOutcome
		return nil
	case "frozen":
		if src.SubmissionRevision == nil {
			return access.ErrConflict
		}
	default:
		return access.ErrConflict
	}
	final, err := q.SubmissionRevision(
		ctx,
		dbgen.SubmissionRevisionParams{
			EventID:  src.EventID,
			TeamID:   src.TeamID,
			Revision: *src.SubmissionRevision,
		},
	)
	if err != nil {
		return failure("original reference", err)
	}
	var document submissions.Document
	if err = json.Unmarshal(final.Document, &document); err != nil {
		return failure("decode original reference", err)
	}
	src.ManagedEvidenceIDs, src.UncapturedEvidenceIDs, src.Audiences = referenceFacts(
		document,
		src.RepositoryURL,
	)
	if len(src.ManagedEvidenceIDs) == 0 && len(src.UncapturedEvidenceIDs) == 0 {
		src.Status = "not_applicable"
	} else if len(src.ManagedEvidenceIDs) == 0 && src.RepositoryID > 0 && src.Status == "unresolved" {
		src.Status = "repository_reference_mismatch"
	}
	return nil
}

func referenceStatus(managed, uncaptured []uuid.UUID) string {
	if len(managed) > 0 {
		return "managed_reference"
	}
	if len(uncaptured) > 0 {
		return "repository_reference_mismatch"
	}
	return "not_applicable"
}
