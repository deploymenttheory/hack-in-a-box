package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

var (
	ErrBlocked  = errors.New("event action has unresolved blockers")
	slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type (
	Issue struct {
		Field   string `json:"field"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	Blocked struct {
		Issues []Issue `json:"issues"`
	}
)

func (b *Blocked) Error() string { return ErrBlocked.Error() }
func (b *Blocked) Unwrap() error { return ErrBlocked }

type Configuration struct {
	Description       string            `json:"description"`
	Timezone          string            `json:"timezone"`
	Venue             string            `json:"venue"`
	Capacity          int               `json:"capacity"`
	MinimumAttendance int               `json:"minimum_attendance"`
	LiveStart         time.Time         `json:"live_start"`
	LiveEnd           time.Time         `json:"live_end"`
	AdmissionCutoff   time.Time         `json:"admission_cutoff"`
	TeamFreeze        time.Time         `json:"team_freeze"`
	SubmissionCutoff  time.Time         `json:"submission_cutoff"`
	DSTChoice         string            `json:"dst_choice"`
	Calendar          schedule.Calendar `json:"calendar"`
	Schedule          []AgendaItem      `json:"schedule"`
}
type AgendaItem struct {
	ID           uuid.UUID `json:"id"`
	Title        string    `json:"title"`
	Audience     []string  `json:"audience"`
	Anchor       string    `json:"anchor"`
	Days         int       `json:"days"`
	Minutes      int       `json:"minutes"`
	BusinessDays bool      `json:"business_days"`
}
type Input struct {
	ID                                uuid.UUID     `json:"id"`
	Name                              string        `json:"name"`
	Slug                              string        `json:"slug"`
	Configuration                     Configuration `json:"configuration"`
	Reason                            string        `json:"reason"`
	ApprovePostponement               bool          `json:"approve_postponement"`
	AcknowledgeCompetitionRestriction bool          `json:"acknowledge_competition_restriction"`
}
type View struct {
	ID                    uuid.UUID     `json:"id"`
	Name                  string        `json:"name"`
	Slug                  string        `json:"slug"`
	OwnerID               *uuid.UUID    `json:"owner_id"`
	Revision              int64         `json:"revision"`
	Stage                 string        `json:"stage"`
	Configuration         Configuration `json:"configuration"`
	ChallengePublication  *time.Time    `json:"challenge_publication"`
	PublishedAt           *time.Time    `json:"published_at"`
	ConfirmedAttendees    int64         `json:"confirmed_attendees"`
	Blockers              []Issue       `json:"blockers"`
	CanPresent            bool          `json:"can_present"`
	CanEdit               bool          `json:"can_edit"`
	CanManageOwner        bool          `json:"can_manage_owner"`
	CanReadStaff          bool          `json:"can_read_staff"`
	CanReadReviewCoverage bool          `json:"can_read_review_coverage"`
	CanReadAttendance     bool          `json:"can_read_attendance"`
	CanReadLogistics      bool          `json:"can_read_logistics"`
	CanTransition         bool          `json:"can_transition"`
}

func DefaultConfiguration() Configuration {
	return Configuration{
		Timezone:          "Europe/London",
		Capacity:          200,
		MinimumAttendance: 30,
		Calendar: schedule.Calendar{
			WorkingDays:   []int{1, 2, 3, 4, 5},
			ExcludedDates: []string{},
		},
		Schedule: []AgendaItem{},
	}
}

func decode(row dbgen.Event) (Configuration, error) {
	c := DefaultConfiguration()
	err := json.Unmarshal(row.Configuration, &c)
	if err != nil {
		return c, fmt.Errorf("decode event settings: %w", err)
	}
	return c, nil
}

func Validate(input Input, complete bool) []Issue {
	issues := []Issue{}
	add := func(field, code, message string) {
		issues = append(issues, Issue{Field: field, Code: code, Message: message})
	}
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 160 {
		add("name", "required", "Enter an event name of 1–160 characters.")
	}
	if len(input.Slug) > 100 || !slugPattern.MatchString(input.Slug) {
		add("slug", "invalid", "Use lowercase letters, numbers and single hyphens.")
	}
	c := input.Configuration
	if len(c.Description) > 4000 || len(c.Venue) > 500 {
		add("description", "too_long", "Shorten the description or venue details.")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil || c.Timezone == "" {
		add("timezone", "invalid", "Choose an IANA timezone, for example Europe/London.")
	}
	if c.Capacity < 1 || c.Capacity > 10000 || c.MinimumAttendance < 1 ||
		c.MinimumAttendance > c.Capacity {
		add(
			"minimum_attendance",
			"invalid",
			"Minimum confirmed attendance must be between one and event capacity.",
		)
	}
	if c.DSTChoice != "" && c.DSTChoice != "earlier" && c.DSTChoice != "later" {
		add("dst_choice", "invalid", "Choose earlier or later for a repeated local time.")
	}
	if err := c.Calendar.Validate(); err != nil {
		add("calendar", "invalid", err.Error())
	}
	issues = append(issues, validateDeadlines(c, complete)...)
	if len(c.Schedule) > 200 {
		add("schedule", "too_many", "Use at most 200 agenda items.")
	}
	ids := map[uuid.UUID]bool{}
	for _, item := range c.Schedule {
		if ids[item.ID] || item.ID == uuid.Nil {
			add("schedule", "duplicate", "Agenda items need distinct identifiers.")
		}
		ids[item.ID] = true
		if _, err := Plan(c, item); err != nil {
			add("schedule", "invalid", err.Error())
		}
	}
	return issues
}

func validateDeadlines(c Configuration, complete bool) []Issue {
	issues := []Issue{}
	add := func(field, code, message string) {
		issues = append(issues, Issue{Field: field, Code: code, Message: message})
	}
	for _, field := range []struct {
		name  string
		value time.Time
	}{{"live_start", c.LiveStart}, {"live_end", c.LiveEnd}, {"admission_cutoff", c.AdmissionCutoff}, {"team_freeze", c.TeamFreeze}, {"submission_cutoff", c.SubmissionCutoff}} {
		if complete && field.value.IsZero() {
			add(field.name, "required", "Set this deadline before opening registration.")
		}
	}
	if !c.LiveStart.IsZero() {
		publication, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
		if err != nil {
			add("live_start", "local_time", err.Error())
		} else if !c.TeamFreeze.IsZero() && !c.TeamFreeze.Before(publication) {
			add("team_freeze", "deadline_order", "Freeze teams before challenge publication.")
		}
		if !c.LiveEnd.IsZero() && !c.LiveEnd.After(c.LiveStart) {
			add("live_end", "deadline_order", "Live end must follow live start.")
		}
		if !c.SubmissionCutoff.IsZero() && c.SubmissionCutoff.Before(c.LiveStart) {
			add(
				"submission_cutoff",
				"deadline_order",
				"Formal submissions open at live start; set a later cutoff.",
			)
		}
	}
	if !c.AdmissionCutoff.IsZero() && !c.TeamFreeze.IsZero() &&
		c.AdmissionCutoff.After(c.TeamFreeze) {
		add("admission_cutoff", "deadline_order", "Close admission no later than team freeze.")
	}
	if !c.LiveEnd.IsZero() && !c.SubmissionCutoff.IsZero() && c.SubmissionCutoff.After(c.LiveEnd) {
		add(
			"submission_cutoff",
			"deadline_order",
			"Submission cutoff must fall within the live hack dates.",
		)
	}
	return issues
}
