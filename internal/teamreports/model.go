// Package teamreports owns deliberately authored, versioned own-team feedback.
// Support feedback, private assessments and public announcements stay separate.
package teamreports

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/results"
)

type Service struct {
	Pool    *pgxpool.Pool
	Now     func() time.Time
	Results *results.Service
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

type (
	Body struct {
		Title      string   `json:"title"`
		Summary    string   `json:"summary"`
		Highlights []string `json:"highlights"`
		NextSteps  []string `json:"next_steps"`
	}
	Source struct {
		ConfirmationID uuid.UUID `json:"confirmation_id"`
		JudgeRunID     uuid.UUID `json:"judge_run_id"`
		Binding        string    `json:"binding"`
		TeamID         uuid.UUID `json:"team_id"`
	}
	Record struct {
		Version            int64                       `json:"version"`
		State              string                      `json:"state"`
		TeamID             uuid.UUID                   `json:"team_id"`
		Body               Body                        `json:"body"`
		IncludeFinalScores bool                        `json:"include_final_scores"`
		Source             Source                      `json:"source"`
		FinalScores        *results.TeamFeedbackSource `json:"final_scores"`
		AuthorID           uuid.UUID                   `json:"author_id"`
		ActorID            uuid.UUID                   `json:"actor_id"`
		Audience           []uuid.UUID                 `json:"audience"`
		ChangedAt          time.Time                   `json:"changed_at"`
		ReleasedAt         *time.Time                  `json:"released_at"`
		Reason             string                      `json:"reason"`
	}
)

type (
	Command struct {
		RequestID          uuid.UUID `json:"request_id"`
		Action             string    `json:"action"`
		SourceBinding      string    `json:"source_binding"`
		Body               Body      `json:"body"`
		IncludeFinalScores bool      `json:"include_final_scores"`
		Reason             string    `json:"reason"`
	}
	Receipt struct {
		RequestID       uuid.UUID `json:"request_id"`
		ActorID         uuid.UUID `json:"actor_id"`
		TeamID          uuid.UUID `json:"team_id"`
		Action          string    `json:"action"`
		ExpectedVersion int64     `json:"expected_version"`
		Version         int64     `json:"version"`
		Fingerprint     string    `json:"fingerprint"`
	}
	Page struct {
		Version       int64                       `json:"version"`
		CurrentSource *results.TeamFeedbackSource `json:"current_source"`
		Latest        *Record                     `json:"latest"`
		Receipt       *Receipt                    `json:"receipt"`
	}
	// Released cannot contain audience member lists, draft prose, rank or awards.
	// Scores are present only after an explicit organiser choice and source check.
	Released struct {
		Version     int64                       `json:"version"`
		TeamID      uuid.UUID                   `json:"team_id"`
		Body        Body                        `json:"body"`
		AuthorID    uuid.UUID                   `json:"author_id"`
		ReleasedBy  uuid.UUID                   `json:"released_by"`
		ReleasedAt  time.Time                   `json:"released_at"`
		FinalScores *results.TeamFeedbackSource `json:"final_scores"`
	}
)
