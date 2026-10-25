package submissionfreeze

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func visibleIDs(ids []uuid.UUID, document submissions.Document) []uuid.UUID {
	allowed := map[uuid.UUID]bool{}
	for _, e := range document.Evidence {
		allowed[e.DeliverableID] = true
	}
	out := []uuid.UUID{}
	for _, id := range ids {
		if allowed[id] {
			out = append(out, id)
		}
	}
	return out
}

// One projection governs selected/history/new/replayed/recovered decisions.
// Current document visibility never grants an older target's code or prose.
func projectDecision(
	document submissions.Document,
	d Decision,
	canonical string,
	organiser bool,
) *Decision {
	managed, _, _ := referenceFacts(document, canonical)
	if len(managed) == 0 {
		return nil
	}
	d.ManagedEvidenceIDs = visibleIDs(d.ManagedEvidenceIDs, document)
	d.UncapturedEvidenceIDs = visibleIDs(d.UncapturedEvidenceIDs, document)
	if !organiser {
		d.Reason = ""
		d.ExternalEvidence = ""
	}
	return &d
}

func (s Service) projectPage(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	v scope,
	out *Page,
) error {
	revisions := []int64{}
	if out.Source.SubmissionRevision != nil {
		revisions = append(revisions, *out.Source.SubmissionRevision)
	}
	for _, d := range out.Decisions {
		revisions = append(revisions, d.SubmissionRevision)
	}
	documents, err := native(
		s,
	).VisibleEvidenceInTransaction(ctx, tx, actor, out.Source.EventID, out.Source.TeamID, revisions)
	if err != nil {
		return failure("visible source targets", err)
	}
	var sourceDocument submissions.Document
	if out.Source.SubmissionRevision != nil {
		sourceDocument = documents[*out.Source.SubmissionRevision]
	}
	managed, _, _ := referenceFacts(sourceDocument, out.Source.RepositoryURL)
	if len(managed) > 0 || v.authority.Member || v.authority.Organiser {
		out.Source.ManagedEvidenceIDs = visibleIDs(out.Source.ManagedEvidenceIDs, sourceDocument)
		out.Source.UncapturedEvidenceIDs = visibleIDs(
			out.Source.UncapturedEvidenceIDs,
			sourceDocument,
		)
	} else {
		out.Source.SubmissionRevision = nil
		out.Source.ManagedEvidenceIDs = []uuid.UUID{}
		out.Source.UncapturedEvidenceIDs = []uuid.UUID{}
		out.Source.Audiences = []string{}
		out.Observations = []Observation{}
	}
	latest := out.Current
	out.Current = nil
	history := []Decision{}
	for _, d := range out.Decisions {
		visible := projectDecision(
			documents[d.SubmissionRevision],
			d,
			out.Source.RepositoryURL,
			v.authority.Organiser,
		)
		if visible != nil {
			history = append(history, *visible)
			if latest != nil && latest.ID == visible.ID {
				copy := *visible
				out.Current = &copy
			}
		}
	}
	out.Decisions = history
	if latest != nil && out.Current == nil {
		out.BindingCurrent = false
		if out.Status == "completed" {
			out.Status = "unavailable"
		}
	}
	return nil
}

func (s Service) projectReceipt(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	v scope,
	out *Receipt,
) error {
	if !v.authority.Member && !v.authority.Organiser {
		out.Branch = ""
	}
	if out.Decision == nil {
		return nil
	}
	row, err := dbgen.New(tx).
		SubmissionFreezeSource(ctx, dbgen.SubmissionFreezeSourceParams{EventID: eventID, TeamID: teamID})
	if err != nil {
		return failure("original receipt source", err)
	}
	var source Source
	if err = json.Unmarshal(row.Definition, &source); err != nil {
		return failure("decode original receipt source", err)
	}
	if source.ID != out.Decision.SourceID {
		return access.ErrConflict
	}
	documents, err := native(
		s,
	).VisibleEvidenceInTransaction(ctx, tx, actor, eventID, teamID, []int64{out.Decision.SubmissionRevision})
	if err != nil {
		return failure("visible original target", err)
	}
	out.Decision = projectDecision(
		documents[out.Decision.SubmissionRevision],
		*out.Decision,
		source.RepositoryURL,
		v.authority.Organiser,
	)
	return nil
}
