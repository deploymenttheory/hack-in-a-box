package reviewdashboard

import (
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func helpMetrics(rows []help.Summary, teamID uuid.UUID, now time.Time) HelpMetrics {
	out := HelpMetrics{Requests: []help.Summary{}}
	examples := []help.Summary{}
	var response, completion int64
	for _, r := range rows {
		if r.TeamID != teamID {
			continue
		}
		switch r.Status {
		case "waiting":
			out.Waiting++
			if r.Overdue {
				out.OverdueWaiting++
			}
			if out.OldestWaitingSeconds == nil || r.WaitingSeconds > *out.OldestWaitingSeconds {
				seconds := r.WaitingSeconds
				out.OldestWaitingSeconds = &seconds
			}
		case "claimed":
			out.Claimed++
		case "resolved":
			out.Resolved++
		case "cancelled":
			out.Cancelled++
		}
		if r.Status == "waiting" || r.Status == "claimed" {
			seconds := max(0, int64(now.Sub(r.RequestedAt)/time.Second))
			if out.OldestOpenSeconds == nil || seconds > *out.OldestOpenSeconds {
				out.OldestOpenSeconds = &seconds
			}
			examples = append(examples, r)
		}
		if r.ResponseSeconds != nil {
			out.FirstResponse.Samples++
			response += *r.ResponseSeconds
		}
		if r.Status == "resolved" && r.CompletionSeconds != nil {
			out.Completion.Samples++
			completion += *r.CompletionSeconds
		}
	}
	out.Unresolved = out.Waiting + out.Claimed
	if out.FirstResponse.Samples > 0 {
		mean := float64(response) / float64(out.FirstResponse.Samples)
		out.FirstResponse.MeanSeconds = &mean
	}
	if out.Completion.Samples > 0 {
		mean := float64(completion) / float64(out.Completion.Samples)
		out.Completion.MeanSeconds = &mean
	}
	slices.SortFunc(examples, func(a, b help.Summary) int {
		if order := a.RequestedAt.Compare(b.RequestedAt); order != 0 {
			return order
		}
		return slices.Compare(a.ID[:], b.ID[:])
	})
	out.MoreRequests = len(examples) > 5
	out.Requests = examples[:min(5, len(examples))]
	return out
}

func addCoverage(out *Completion, cp scorecards.CheckpointCoverage) {
	out.Checkpoints++
	for _, review := range cp.Reviews {
		out.RequiredReviews++
		switch review.State {
		case "missing_assignment":
			out.MissingAssignment++
		case "missing":
			out.Missing++
		case "draft":
			out.Drafts++
		case "submitted":
			out.Submitted++
		case "not_open":
			out.NotOpen++
		default:
			out.Unavailable++
		}
		if review.PendingCategories != nil {
			out.PendingCategories += *review.PendingCategories
		}
		if review.MissingEvidence != nil {
			out.MissingEvidence += *review.MissingEvidence
		}
		if review.Overdue {
			out.OverdueCheckins++
		}
	}
	out.Complete = out.RequiredReviews > 0 && out.Submitted == out.RequiredReviews &&
		out.PendingCategories == 0 &&
		out.MissingEvidence == 0
}
