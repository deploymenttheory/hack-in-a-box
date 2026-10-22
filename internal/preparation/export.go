package preparation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// Report contains deliberately small export fields, never raw registration,
// task evidence, private drafts, review reasons or configuration documents.
type Report struct {
	Event       string
	Audience    string
	Timezone    string
	GeneratedAt time.Time
	Rows        []ReportRow
}
type ReportRow struct{ Kind, Name, Status, Due, Detail string }

func (r *Report) add(kind, name, status, due, detail string) {
	r.Rows = append(r.Rows, ReportRow{kind, name, status, due, detail})
}

// Export holds the event lock while resolving every audience and resource grant.
// It cannot combine an old role with a newer team assignment or withdrawal.
func (s Service) Export(
	ctx context.Context,
	actor, eventID uuid.UUID,
	audience string,
) (Report, error) {
	result := Report{Audience: audience, GeneratedAt: s.now().UTC(), Rows: []ReportRow{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock report event: %w", err)
		}
		roles, err := authority(ctx, q, actor, eventID)
		if err != nil {
			return err
		}
		roles, err = exportRoles(roles, audience)
		if err != nil {
			return err
		}
		c := event.DefaultConfiguration()
		if err = json.Unmarshal(row.Configuration, &c); err != nil {
			return fmt.Errorf("decode report event: %w", err)
		}
		result.Event, result.Timezone = row.Name, c.Timezone
		loc, err := time.LoadLocation(c.Timezone)
		if err != nil {
			return fmt.Errorf("report timezone: %w", err)
		}
		local := func(t time.Time) string {
			if t.IsZero() {
				return "Not configured"
			}
			return t.In(loc).Format("2006-01-02 15:04:05 MST (-07:00)")
		}
		reportDates(&result, row, c, local)
		tasks, err := s.page(ctx, q, actor, row, roles, Filter{})
		if err != nil {
			return err
		}
		reportTasks(&result, tasks, local)
		if audience == "organiser" {
			if err = operationalCounts(ctx, q, eventID, c, &result); err != nil {
				return err
			}
		}
		entries, err := q.SubmissionEntries(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read report entries: %w", err)
		}
		service := submissions.Service{
			Pool:  s.Pool,
			Clock: func() time.Time { return result.GeneratedAt },
		}
		missing := 0
		for _, entry := range entries {
			if !entry.Active || entry.TeamState == "disbanded" {
				continue
			}
			p, readErr := service.Report(ctx, tx, actor, eventID, entry.TeamID, audience)
			if errors.Is(readErr, access.ErrForbidden) {
				continue
			}
			if readErr != nil {
				return fmt.Errorf("read report submission: %w", readErr)
			}
			result.add(
				"submission",
				entry.TeamName,
				p.Outcome,
				"",
				"Explicit submission outcome; not a score",
			)
			// Challenge definitions are embargoed until actual publication for attendees.
			if audience != "organiser" &&
				(!row.PublishedAt.Valid || result.GeneratedAt.Before(row.PublishedAt.Time)) {
				continue
			}
			supplied := map[uuid.UUID]bool{}
			if p.Final != nil {
				for _, e := range p.Final.Evidence {
					supplied[e.DeliverableID] = true
				}
			}
			for _, d := range p.Definitions {
				status := "Optional"
				if d.Required {
					status = "Missing formal evidence"
					missing++
				}
				if supplied[d.ID] {
					status = "Submitted"
					if d.Required {
						missing--
					}
				}
				result.add("deliverable", entry.TeamName+": "+d.Title, status, local(d.DueAt), "")
			}
		}
		result.add(
			"count",
			"Visible missing required deliverables",
			strconv.Itoa(missing),
			"",
			"Private working drafts do not count as formal evidence",
		)
		return nil
	})
	if err != nil {
		return Report{}, fmt.Errorf("export readiness: %w", err)
	}
	return result, nil
}

func operationalCounts(
	ctx context.Context,
	q *dbgen.Queries,
	id uuid.UUID,
	c event.Configuration,
	r *Report,
) error {
	confirmed, err := q.ConfirmedAttendees(ctx, id)
	if err != nil {
		return fmt.Errorf("report competitor minimum: %w", err)
	}
	facts, err := q.ConfirmedTeamPeople(ctx, id)
	if err != nil {
		return fmt.Errorf("report confirmed competitors: %w", err)
	}
	staff, err := q.AttendanceStaff(ctx, id)
	if err != nil {
		return fmt.Errorf("report staff: %w", err)
	}
	reserved, err := q.ReservedAttendees(ctx, id)
	if err != nil {
		return fmt.Errorf("report capacity: %w", err)
	}
	r.add(
		"count",
		"Confirmed competitors",
		strconv.FormatInt(confirmed, 10),
		"",
		fmt.Sprintf("Minimum required: %d", c.MinimumAttendance),
	)
	members, err := q.HackTeamMembers(ctx, id)
	if err != nil {
		return fmt.Errorf("report assignments: %w", err)
	}
	assigned := map[uuid.UUID]bool{}
	for _, m := range members {
		if m.Snapshot == "published" {
			assigned[m.PersonID] = true
		}
	}
	unassigned := 0
	for _, p := range facts {
		if !assigned[p.ID] {
			unassigned++
		}
	}
	r.add(
		"count",
		"Confirmed competitors without published teams",
		strconv.Itoa(unassigned),
		"",
		"",
	)
	r.add(
		"count",
		"Staff",
		strconv.Itoa(len(staff)),
		"",
		"Staff do not contribute to the competitor minimum",
	)
	overlap := 0
	for _, p := range staff {
		if p.Competing {
			overlap++
		}
	}
	r.add(
		"count",
		"Staff also competing",
		strconv.Itoa(overlap),
		"",
		"Already included in confirmed competitors; do not count twice",
	)
	r.add(
		"count",
		"Unaccepted reservations",
		strconv.FormatInt(reserved, 10),
		"",
		"Not confirmed competitors",
	)
	return nil
}

func reportDates(
	result *Report,
	row dbgen.Event,
	c event.Configuration,
	local func(time.Time) string,
) {
	audience := result.Audience
	for _, m := range []struct {
		name string
		at   time.Time
	}{{"Admission cutoff", c.AdmissionCutoff}, {"Team freeze", c.TeamFreeze}, {"Live start", c.LiveStart}, {"Submission cutoff", c.SubmissionCutoff}, {"Live end", c.LiveEnd}} {
		result.add("milestone", m.name, "", local(m.at), "")
	}
	planned, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
	if c.LiveStart.IsZero() || err != nil {
		result.add("milestone", "Challenge publication", "Not configured", "", "")
	} else {
		status := "Planned"
		if row.PublishedAt.Valid {
			status = "Released at " + local(row.PublishedAt.Time)
		}
		result.add(
			"milestone",
			"Challenge publication",
			status,
			local(planned),
			"Seven days before live start",
		)
	}
	for _, a := range c.Schedule {
		role := audience
		if role == "participant" {
			role = "attendee"
		}
		if !slices.Contains(a.Audience, role) {
			continue
		}
		at, planErr := event.Plan(c, a)
		if planErr != nil {
			result.add("agenda", a.Title, "Scheduling issue", "", "")
		} else {
			result.add("agenda", a.Title, "", local(at), "")
		}
	}
}

func reportTasks(result *Report, tasks Page, local func(time.Time) string) {
	overdue, unowned := 0, 0
	for _, t := range tasks.Items {
		status := t.Definition.Status
		if t.Overdue {
			status += "; overdue"
			overdue++
		}
		if t.Blocked {
			status += "; blocked"
		}
		if t.Unowned {
			status += "; unassigned"
			unowned++
		}
		due := ""
		if t.DueAt != nil {
			due = local(*t.DueAt)
		}
		result.add(
			"task",
			t.Definition.Title,
			status,
			due,
			"Owner role: "+t.Definition.OwnerRole,
		)
	}
	result.add("count", "Visible overdue tasks", strconv.Itoa(overdue), "", "")
	result.add("count", "Visible unassigned tasks", strconv.Itoa(unowned), "", "")
}

func exportRoles(roles []string, audience string) ([]string, error) {
	role := audience
	if audience == "participant" {
		role = "attendee"
	}
	if !slices.Contains([]string{"organiser", "mentor", "participant"}, audience) {
		return nil, access.ErrInvalid
	}
	if !slices.Contains(roles, role) {
		return nil, access.ErrForbidden
	}
	return []string{role}, nil
}
