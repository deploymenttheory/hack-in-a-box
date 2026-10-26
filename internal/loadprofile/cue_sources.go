package loadprofile

import "github.com/google/uuid"

type CueSourceReport struct {
	CommittedSources   int `json:"committed_sources"`
	PublishedSources   int `json:"tracked_published_sources"`
	ExpectedReceipts   int `json:"expected_receipts"`
	MatchedReceipts    int `json:"matched_receipts"`
	Unreconciled       int `json:"unreconciled"`
	Changed            int `json:"changed"`
	DuplicateEffects   int `json:"duplicate_effects"`
	MissingReceipts    int `json:"missing_receipts"`
	MismatchedReceipts int `json:"mismatched_receipts"`
	InvalidClockOrder  int `json:"invalid_clock_order"`
}

func (r CueSourceReport) Complete() bool {
	return r.CommittedSources > 0 && r.CommittedSources == r.PublishedSources &&
		r.ExpectedReceipts >= r.CommittedSources && r.MatchedReceipts == r.ExpectedReceipts &&
		r.Unreconciled == 0 && r.Changed == 0 && r.DuplicateEffects == 0 &&
		r.MissingReceipts == 0 && r.MismatchedReceipts == 0 && r.InvalidClockOrder == 0
}

// CueSources reconciles ALL sent cue candidates with independent native
// decision/audit facts and display receipts. A committed cue whose original
// response was lost cannot vanish from the SSE oracle or become an inferred
// original ACK. It remains committed-but-untracked and prevents acceptance.
func CueSources(
	specs map[uuid.UUID]WriteSpec,
	facts map[uuid.UUID]EffectFact,
	display DisplayReport,
) CueSourceReport {
	report := CueSourceReport{
		PublishedSources: display.Publications,
		ExpectedReceipts: display.ExpectedReceipts, MatchedReceipts: display.MatchedReceipts,
		MissingReceipts: display.MissingReceipts, MismatchedReceipts: display.MismatchedReceipts,
		InvalidClockOrder: display.InvalidClockOrder,
	}
	unique := make(map[string]bool)
	for id, spec := range specs {
		if spec.Kind != CueEffect {
			continue
		}
		fact, exists := facts[id]
		if !exists || fact.Spec != spec {
			report.Unreconciled++
			continue
		}
		if !fact.Present {
			continue
		}
		if fact.Fingerprint != spec.Fingerprint {
			report.Changed++
		}
		if fact.Effects != 1 {
			report.DuplicateEffects++
		}
		if !unique[spec.identity()] {
			unique[spec.identity()] = true
			report.CommittedSources++
		}
	}
	return report
}
