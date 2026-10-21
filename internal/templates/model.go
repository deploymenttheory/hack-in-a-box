// Package templates owns immutable reusable configuration snapshots.
package templates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

type (
	Condition = forms.Condition
	Field     = forms.Field
	Form      = forms.Definition
	Task      struct {
		ID         uuid.UUID   `json:"id"`
		Title      string      `json:"title"`
		OwnerRole  string      `json:"owner_role"`
		DependsOn  []uuid.UUID `json:"depends_on"`
		ScheduleID *uuid.UUID  `json:"schedule_id,omitempty"`
	}
	Deliverable struct {
		ID          uuid.UUID  `json:"id"`
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Required    bool       `json:"required"`
		ScheduleID  *uuid.UUID `json:"schedule_id,omitempty"`
	}
	RoleBrief struct {
		ID   uuid.UUID `json:"id"`
		Role string    `json:"role"`
		Body string    `json:"body"`
	}
	Criterion struct {
		ID       uuid.UUID `json:"id"`
		Label    string    `json:"label"`
		Guidance string    `json:"guidance"`
		MaxScore int       `json:"max_score"`
		Weight   int       `json:"weight"`
	}
	Rubric struct {
		ID       uuid.UUID   `json:"id"`
		Name     string      `json:"name"`
		Criteria []Criterion `json:"criteria"`
	}
	Message struct {
		ID         uuid.UUID  `json:"id"`
		Name       string     `json:"name"`
		Subject    string     `json:"subject"`
		Body       string     `json:"body"`
		Audience   []string   `json:"audience"`
		ScheduleID *uuid.UUID `json:"schedule_id,omitempty"`
	}
	Asset struct {
		ID        uuid.UUID `json:"id"`
		Kind      string    `json:"kind"`
		MediaType string    `json:"media_type"`
		Content   []byte    `json:"content"`
	}
	Branding struct {
		Primary    string  `json:"primary"`
		Accent     string  `json:"accent"`
		Background string  `json:"background"`
		Foreground string  `json:"foreground"`
		Assets     []Asset `json:"assets"`
	}
	Definitions struct {
		Forms        []Form        `json:"forms"`
		Tasks        []Task        `json:"tasks"`
		Deliverables []Deliverable `json:"deliverables"`
		RoleBriefs   []RoleBrief   `json:"role_briefs"`
		Rubrics      []Rubric      `json:"rubrics"`
		Messages     []Message     `json:"messages"`
		Branding     Branding      `json:"branding"`
	}
	Bundle struct {
		Event       event.Configuration `json:"event"`
		Definitions Definitions         `json:"definitions"`
	}
	Input struct {
		ID            uuid.UUID `json:"id"`
		Name          string    `json:"name"`
		Configuration Bundle    `json:"configuration"`
	}
	CloneInput struct {
		ID                                uuid.UUID `json:"id"`
		Name                              string    `json:"name"`
		Slug                              string    `json:"slug"`
		LiveStart                         time.Time `json:"live_start"`
		Timezone                          string    `json:"timezone"`
		DSTChoice                         string    `json:"dst_choice"`
		Version                           int32     `json:"version"`
		AcknowledgeCompetitionRestriction bool      `json:"acknowledge_competition_restriction"`
	}
	View struct {
		ID               uuid.UUID `json:"id"`
		Name             string    `json:"name"`
		Revision         int64     `json:"revision"`
		LatestVersion    int32     `json:"latest_version"`
		PublishedVersion *int32    `json:"published_version"`
		Version          int32     `json:"version"`
		Configuration    Bundle    `json:"configuration"`
	}
)

func EmptyDefinitions() Definitions {
	return Definitions{
		Forms:        []Form{},
		Tasks:        []Task{},
		Deliverables: []Deliverable{},
		RoleBriefs:   []RoleBrief{},
		Rubrics:      []Rubric{},
		Messages:     []Message{},
		Branding: Branding{
			Primary:    "#173a35",
			Accent:     "#d17c32",
			Background: "#f8f9f5",
			Foreground: "#173a35",
			Assets:     []Asset{},
		},
	}
}

var (
	colour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	roles  = []string{"attendee", "organiser", "mentor", "judge", "mc", "logistics", "photographer"}
)

func Validate(bundle Bundle) error {
	if issues := event.Validate(
		event.Input{Name: "Template", Slug: "template", Configuration: bundle.Event},
		false,
	); len(
		issues,
	) > 0 {
		return &event.Blocked{Issues: issues}
	}
	d := bundle.Definitions
	if err := validateCounts(d); err != nil {
		return err
	}
	ids := map[uuid.UUID]bool{}
	valid := true
	visitIDs(&bundle, func(id *uuid.UUID) {
		if *id == uuid.Nil || ids[*id] {
			valid = false
		}
		ids[*id] = true
	})
	if !valid {
		return access.ErrInvalid
	}
	for _, form := range d.Forms {
		if err := validateForm(form); err != nil {
			return err
		}
	}
	if err := validateTasks(d.Tasks); err != nil {
		return err
	}
	agenda := map[uuid.UUID]bool{}
	for _, item := range bundle.Event.Schedule {
		agenda[item.ID] = true
	}
	linked := func(id *uuid.UUID) bool { return id == nil || agenda[*id] }
	for _, task := range d.Tasks {
		if !linked(task.ScheduleID) {
			return access.ErrInvalid
		}
	}
	for _, item := range d.Deliverables {
		if item.Title == "" || len(item.Title) > 160 || len(item.Description) > 4000 ||
			!linked(item.ScheduleID) {
			return access.ErrInvalid
		}
	}
	for _, brief := range d.RoleBriefs {
		if !slices.Contains(roles, brief.Role) || len(brief.Body) > 10000 {
			return access.ErrInvalid
		}
	}
	for _, rubric := range d.Rubrics {
		if err := validateRubric(rubric); err != nil {
			return err
		}
	}
	for _, message := range d.Messages {
		if err := validateMessage(message); err != nil {
			return err
		}
		if !linked(message.ScheduleID) {
			return access.ErrInvalid
		}
	}
	return validateBranding(d.Branding)
}

func validateForm(form Form) error {
	if err := forms.Validate(form); err != nil {
		return fmt.Errorf("validate template form: %w", err)
	}
	return nil
}

func validateTasks(tasks []Task) error {
	graph := map[uuid.UUID][]uuid.UUID{}
	for _, task := range tasks {
		if task.Title == "" || len(task.Title) > 160 || !slices.Contains(roles, task.OwnerRole) {
			return access.ErrInvalid
		}
		graph[task.ID] = task.DependsOn
	}
	state := map[uuid.UUID]int{}
	var walk func(uuid.UUID) bool
	walk = func(id uuid.UUID) bool {
		if state[id] == 1 {
			return false
		}
		if state[id] == 2 {
			return true
		}
		dependencies, ok := graph[id]
		if !ok {
			return false
		}
		state[id] = 1
		for _, dependency := range dependencies {
			if !walk(dependency) {
				return false
			}
		}
		state[id] = 2
		return true
	}
	for id := range graph {
		if !walk(id) {
			return access.ErrInvalid
		}
	}
	return nil
}

func validateRubric(r Rubric) error {
	if r.Name == "" || len(r.Name) > 160 || len(r.Criteria) > 50 {
		return access.ErrInvalid
	}
	for _, c := range r.Criteria {
		if c.Label == "" || len(c.Label) > 160 || len(c.Guidance) > 4000 || c.MaxScore < 1 ||
			c.MaxScore > 1000 ||
			c.Weight < 1 ||
			c.Weight > 100 {
			return access.ErrInvalid
		}
	}
	return nil
}

func validateMessage(m Message) error {
	if m.Name == "" || len(m.Name) > 160 || len(m.Subject) > 200 || len(m.Body) > 10000 ||
		len(m.Audience) == 0 ||
		len(m.Audience) > 8 {
		return access.ErrInvalid
	}
	for _, role := range m.Audience {
		if role != "all" && !slices.Contains(roles, role) {
			return access.ErrInvalid
		}
	}
	return nil
}

func visitIDs(b *Bundle, visit func(*uuid.UUID)) {
	for i := range b.Event.Schedule {
		visit(&b.Event.Schedule[i].ID)
	}
	for i := range b.Definitions.Forms {
		f := &b.Definitions.Forms[i]
		visit(&f.ID)
		for j := range f.Fields {
			visit(&f.Fields[j].ID)
		}
	}
	for i := range b.Definitions.Tasks {
		visit(&b.Definitions.Tasks[i].ID)
	}
	for i := range b.Definitions.Deliverables {
		visit(&b.Definitions.Deliverables[i].ID)
	}
	for i := range b.Definitions.RoleBriefs {
		visit(&b.Definitions.RoleBriefs[i].ID)
	}
	for i := range b.Definitions.Rubrics {
		r := &b.Definitions.Rubrics[i]
		visit(&r.ID)
		for j := range r.Criteria {
			visit(&r.Criteria[j].ID)
		}
	}
	for i := range b.Definitions.Messages {
		visit(&b.Definitions.Messages[i].ID)
	}
	for i := range b.Definitions.Branding.Assets {
		visit(&b.Definitions.Branding.Assets[i].ID)
	}
}

func Remap(source Bundle, input CloneInput) (Bundle, error) {
	var result Bundle
	if input.ID == uuid.Nil {
		return result, access.ErrInvalid
	}
	if err := Validate(source); err != nil {
		return result, err
	}
	data, err := json.Marshal(source)
	if err != nil {
		return result, fmt.Errorf("copy template: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("decode template copy: %w", err)
	}
	mapping := map[uuid.UUID]uuid.UUID{}
	visitIDs(
		&result,
		func(id *uuid.UUID) { old := *id; *id = uuid.NewSHA1(input.ID, old[:]); mapping[old] = *id },
	)
	for i := range result.Definitions.Forms {
		for j := range result.Definitions.Forms[i].Fields {
			condition := result.Definitions.Forms[i].Fields[j].VisibleWhen
			if condition != nil {
				condition.FieldID = mapping[condition.FieldID]
			}
		}
	}
	link := func(id **uuid.UUID) {
		if *id != nil {
			value := mapping[**id]
			*id = &value
		}
	}
	for i := range result.Definitions.Tasks {
		task := &result.Definitions.Tasks[i]
		for j := range task.DependsOn {
			task.DependsOn[j] = mapping[task.DependsOn[j]]
		}
		link(&task.ScheduleID)
	}
	for i := range result.Definitions.Deliverables {
		link(&result.Definitions.Deliverables[i].ScheduleID)
	}
	for i := range result.Definitions.Messages {
		link(&result.Definitions.Messages[i].ScheduleID)
	}
	if err := rebase(&result.Event, input); err != nil {
		return Bundle{}, err
	}
	if err := Validate(result); err != nil {
		return Bundle{}, err
	}
	return result, nil
}

func rebase(c *event.Configuration, input CloneInput) error {
	if input.LiveStart.IsZero() {
		return access.ErrInvalid
	}
	oldZone, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return access.ErrInvalid
	}
	zone := input.Timezone
	if zone == "" {
		zone = c.Timezone
	}
	newZone, err := time.LoadLocation(zone)
	if err != nil {
		return access.ErrInvalid
	}
	naive := func(t time.Time, loc *time.Location) time.Time {
		v := t.In(loc)
		return time.Date(
			v.Year(),
			v.Month(),
			v.Day(),
			v.Hour(),
			v.Minute(),
			v.Second(),
			0,
			time.UTC,
		)
	}
	oldStart, newStart := naive(c.LiveStart, oldZone), naive(input.LiveStart, newZone)
	for _, field := range []*time.Time{&c.LiveEnd, &c.AdmissionCutoff, &c.TeamFreeze, &c.SubmissionCutoff} {
		if field.IsZero() {
			continue
		}
		wall := newStart.Add(naive(*field, oldZone).Sub(oldStart)).Format("2006-01-02T15:04:05")
		resolved, err := schedule.ResolveLocal(wall, zone, input.DSTChoice)
		if err != nil {
			return fmt.Errorf("rebase template dates: %w", err)
		}
		*field = resolved
	}
	c.LiveStart = input.LiveStart
	c.Timezone = zone
	c.DSTChoice = input.DSTChoice
	return nil
}

func validateCounts(d Definitions) error {
	if len(d.Forms) > 50 || len(d.Tasks) > 200 || len(d.Deliverables) > 100 ||
		len(d.RoleBriefs) > 20 ||
		len(d.Rubrics) > 20 ||
		len(d.Messages) > 200 ||
		len(d.Branding.Assets) > 3 {
		return access.ErrInvalid
	}
	return nil
}
