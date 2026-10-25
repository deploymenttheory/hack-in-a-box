package results

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

type assessmentSource struct {
	preview    Preview
	completion *judging.Completion
}

func emptyPreview() Preview {
	return Preview{Tallies: []TeamTally{}, Ties: []Tie{}, Blockers: []string{}}
}

func digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s Service) source(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) (assessmentSource, error) {
	return s.sourceMode(ctx, tx, actor, eventID, false)
}

func (s Service) sourceMode(ctx context.Context, tx pgx.Tx, actor, eventID uuid.UUID, readOnly bool) (assessmentSource, error) {
	out := assessmentSource{preview: emptyPreview()}
	q := dbgen.New(tx)
	runs, err := q.CurrentRubricRuns(ctx, eventID)
	if err != nil {
		return out, failure("current assessment runs", err)
	}
	var selected *dbgen.RubricAssessmentRun
	for i := range runs {
		if runs[i].Purpose == "judge_final" {
			selected = &runs[i]
			out.preview.JudgeRunID = selected.ID
			break
		}
	}
	if selected == nil {
		for i := range runs {
			if runs[i].Purpose == "mentor_final" {
				selected = &runs[i]
				break
			}
		}
	}
	if selected == nil {
		out.preview.Blockers = append(out.preview.Blockers, "current_assessment_run_required")
		return out, nil
	}
	if err = rubric.ValidateRunPublicationInTransaction(ctx, tx, eventID, selected.ID); err != nil {
		if errors.Is(err, access.ErrConflict) || errors.Is(err, access.ErrForbidden) {
			out.preview.Blockers = append(out.preview.Blockers, "current_published_rubric_required")
			return out, nil
		}
		return out, failure("current rubric publication", err)
	}
	version, err := q.RubricVersion(
		ctx,
		dbgen.RubricVersionParams{
			EventID:  eventID,
			RubricID: selected.RubricID,
			Version:  selected.RubricVersion,
		},
	)
	if err != nil {
		return out, failure("published criteria", err)
	}
	out.preview.Reference = rubric.Reference{
		ID:      selected.RubricID,
		Version: selected.RubricVersion,
		Digest:  version.PublicDigest,
	}
	if err = json.Unmarshal(version.PublicDocument, &out.preview.Criteria); err != nil {
		return out, failure("decode public criteria", err)
	}
	if out.preview.Criteria.Mode == rubric.MentorQualification {
		if s.Qualification == nil {
			out.preview.Blockers = append(
				out.preview.Blockers,
				"current_signed_qualification_required",
			)
		} else {
			selection, err := s.Qualification.SelectionInTransaction(ctx, tx, eventID)
			switch {
			case err == nil && selection.Reference == out.preview.Reference:
				out.preview.Selection = &selection
				out.preview.FinalistsBinding = digest(selection)
			case err == nil,
				errors.Is(err, access.ErrConflict),
				errors.Is(err, access.ErrForbidden),
				errors.Is(err, access.ErrNotFound):
				out.preview.Blockers = append(
					out.preview.Blockers,
					"current_signed_qualification_required",
				)
			default:
				return out, failure("current native qualification selection", err)
			}
		}
	}
	if out.preview.JudgeRunID == uuid.Nil {
		out.preview.Blockers = append(out.preview.Blockers, "current_judge_final_run_required")
		return out, nil
	}
	if s.Judging == nil {
		out.preview.Blockers = append(
			out.preview.Blockers,
			"current_required_judge_assessment_needed",
		)
		return out, nil
	}
	completionReader := s.Judging.CompletionInTransaction
	if readOnly {
		completionReader = s.Judging.ReadCompletionInTransaction
	}
	completion, tallies, err := calculateCompletion(ctx, tx, completionReader, actor, eventID, out.preview.JudgeRunID)
	if err != nil {
		if errors.Is(err, access.ErrConflict) || errors.Is(err, access.ErrForbidden) ||
			errors.Is(err, access.ErrNotFound) ||
			errors.Is(err, ErrIncomplete) || readOnly && errors.Is(err, event.ErrBlocked) {
			out.preview.Blockers = append(
				out.preview.Blockers,
				"current_required_judge_assessment_needed",
			)
			return out, nil
		}
		return out, failure("current independent judge source", err)
	}
	if !completion.Deliberation {
		out.preview.Blockers = append(out.preview.Blockers, "judge_deliberation_required")
		return out, nil
	}
	out.completion = &completion
	out.preview.Tallies = tallies
	qualBinding := ""
	if out.preview.Selection != nil {
		qualBinding = out.preview.FinalistsBinding
	}
	out.preview.Binding = digest(struct {
		RunID                uuid.UUID
		Reference            rubric.Reference
		JudgeBinding         string
		QualificationBinding string
	}{completion.RunID, completion.Reference, completion.Binding, qualBinding})
	out.preview.Ties, err = sourceTies(out.preview)
	return out, err
}
