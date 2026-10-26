package loadprofile

import "time"

// Host records both driver and service placement; Docker limits are part of
// the baseline, not inferred from the host's advertised CPU/RAM.
type Host struct {
	OS                string            `json:"os"`
	CPU               string            `json:"cpu"`
	LogicalCPUs       int               `json:"logical_cpus"`
	MemoryBytes       uint64            `json:"memory_bytes"`
	DockerVersion     string            `json:"docker_version"`
	DockerCPUs        int               `json:"docker_cpus"`
	DockerMemoryBytes uint64            `json:"docker_memory_bytes"`
	DriverPlacement   string            `json:"driver_placement"`
	ServiceLimits     map[string]string `json:"service_limits"`
}

type Dataset struct {
	Events                       int    `json:"events"`
	TargetPeoplePerEvent         int    `json:"target_people_per_event"`
	PeoplePerEvent               []int  `json:"people_per_event"`
	StaffPerEvent                []int  `json:"staff_per_event"`
	RegistrationCapacityPerEvent []int  `json:"registration_capacity_per_event"`
	ConfirmedPerEvent            []int  `json:"confirmed_per_event"`
	AuthenticatedSessions        int    `json:"authenticated_sessions"`
	Mentors                      int    `json:"mentors"`
	DisplayStreams               int    `json:"display_streams"`
	Seed                         uint64 `json:"seed"`
}

type Oracle struct {
	Name           string `json:"name"`
	Passed         bool   `json:"passed"`
	Observations   uint64 `json:"observations"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

var requiredOracles = [...]string{
	"registration-capacity-fifo",
	"accepted-write-reconciliation",
	"current-permission-private-search",
	"single-help-claim-owner",
	"scheduled-delivery-deduplication",
	"approved-followup-current-authority",
	"sse-current-source",
}

// RequiredOracleNames returns a copy of the fixed release oracle names.
func RequiredOracleNames() []string {
	return append([]string(nil), requiredOracles[:]...)
}

type Report struct {
	Source             string            `json:"source"`
	Tree               string            `json:"tree"`
	Images             map[string]string `json:"images"`
	Schema             int               `json:"schema"`
	Profile            string            `json:"profile"`
	Started            time.Time         `json:"started"`
	Finished           time.Time         `json:"finished"`
	Elapsed            time.Duration     `json:"monotonic_elapsed_ns"`
	Host               Host              `json:"host"`
	Dataset            Dataset           `json:"dataset"`
	Activity           ActivityReport    `json:"activity"`
	Operations         []OperationReport `json:"operations"`
	Integrity          IntegrityReport   `json:"integrity"`
	Attempts           AttemptReport     `json:"write_attempts"`
	Oracles            []Oracle          `json:"oracles"`
	ResourceSamples    int               `json:"resource_samples"`
	QueueSamples       int               `json:"queue_samples"`
	BurstCompleted     bool              `json:"burst_completed"`
	SustainedCompleted bool              `json:"sustained_completed"`
	ExternalTLS        bool              `json:"external_tls"`
	DurableVolumes     bool              `json:"durable_volumes"`
	WorkerWallClock    bool              `json:"worker_wall_clock"`
}

type Evaluation struct {
	FullLoad       bool     `json:"full_load"`
	Passed         bool     `json:"passed"`
	Failures       []string `json:"failures"`
	UnexpectedRate float64  `json:"unexpected_error_rate"`
}

// Evaluate never upgrades a diagnostic run into release evidence. Performance
// and integrity failures remain separate from whether the full profile ran.
// It consumes measured facts; the runner must supply real monotonic elapsed
// time, native session activity and independently reconciled domain oracles.
func (r Report) Evaluate() Evaluation {
	e := Evaluation{Failures: []string{}}
	fail := func(reason string) { e.Failures = append(e.Failures, reason) }
	shape := r.Profile == "release" && r.Elapsed >= 30*time.Minute && r.Dataset.Events == 5 &&
		r.Dataset.TargetPeoplePerEvent == 200 &&
		r.Activity.ActiveEveryMinute >= 1000 &&
		r.Activity.Windows >= 30 &&
		r.BurstCompleted && r.SustainedCompleted
	e.FullLoad = shape
	if !shape {
		fail(
			"complete five-event/1000-active-session burst and real 30-minute sustained profile not proven",
		)
	}
	r.validateBaseline(fail)
	r.validateDataset(fail)
	r.validateWriteIntegrity(fail)
	e.UnexpectedRate = r.evaluateOperations(fail)
	r.validateOracles(fail)
	e.Passed = len(e.Failures) == 0
	return e
}

func (r Report) validateBaseline(fail func(string)) {
	if r.Source == "" || r.Tree == "" || r.Schema == 0 || len(r.Images) < 4 {
		fail("runtime source/image/schema provenance incomplete")
	}
	if r.Started.IsZero() || r.Finished.Before(r.Started) || r.Finished.Sub(r.Started) < r.Elapsed {
		fail("wall and monotonic stage times inconsistent")
	}
	if r.Host.OS == "" || r.Host.CPU == "" || r.Host.LogicalCPUs < 1 || r.Host.MemoryBytes == 0 ||
		r.Host.DockerVersion == "" ||
		r.Host.DockerCPUs < 1 ||
		r.Host.DockerMemoryBytes == 0 ||
		r.Host.DriverPlacement == "" ||
		len(r.Host.ServiceLimits) < 4 {
		fail("baseline host and service resource limits incomplete")
	}
	if !r.ExternalTLS || !r.DurableVolumes || !r.WorkerWallClock {
		fail("external TLS/durable-volume/default-wall-worker baseline not proven")
	}
}

func (r Report) validateDataset(fail func(string)) {
	if len(r.Dataset.ConfirmedPerEvent) != 5 || len(r.Dataset.PeoplePerEvent) != 5 ||
		len(r.Dataset.StaffPerEvent) != 5 ||
		len(r.Dataset.RegistrationCapacityPerEvent) != 5 {
		fail("five-event committed admission census missing")
	} else {
		for index, confirmed := range r.Dataset.ConfirmedPerEvent {
			if r.Dataset.PeoplePerEvent[index] != 200 || r.Dataset.StaffPerEvent[index] < 0 ||
				confirmed < 1 ||
				confirmed+r.Dataset.StaffPerEvent[index] != r.Dataset.PeoplePerEvent[index] ||
				confirmed != r.Dataset.RegistrationCapacityPerEvent[index] {
				fail(
					"per-event 200-person cohort and actual full competitor/staff capacity census inconsistent",
				)
				break
			}
		}
	}
	if r.Activity.Sessions < 1000 || r.Activity.MissingActorMinutes != 0 ||
		r.Dataset.AuthenticatedSessions != r.Activity.Sessions {
		fail("continuous authenticated actor activity incomplete")
	}
	if r.Dataset.Mentors < 5 || r.Dataset.DisplayStreams < 5 {
		fail("concurrent mentor and display cohort incomplete")
	}
	if r.ResourceSamples < 30 || r.QueueSamples < 30 {
		fail("resource utilization or queue lag time series incomplete")
	}
}

func (r Report) validateWriteIntegrity(fail func(string)) {
	if r.Integrity.Accepted == 0 || r.Integrity.Accepted != r.Integrity.Committed ||
		r.Integrity.Missing != 0 ||
		r.Integrity.Changed != 0 ||
		r.Integrity.DuplicateEffects != 0 {
		fail("accepted native write reconciliation failed")
	}
	if r.Attempts.Sent == 0 || r.Attempts.OriginalAcknowledgements == 0 ||
		r.Attempts.Sent != r.Attempts.OriginalAcknowledgements+r.Attempts.ExplicitConflicts+r.Attempts.UnknownOriginalAcknowledgements ||
		r.Attempts.Unreconciled != 0 ||
		r.Attempts.MissingAcknowledgedEffects != 0 ||
		r.Attempts.ChangedEffects != 0 ||
		r.Attempts.DuplicateEffects != 0 ||
		r.Attempts.CommittedDespiteConflict != 0 {
		fail("complete sent-write/original-response/effect reconciliation failed")
	}
}

func (r Report) evaluateOperations(fail func(string)) float64 {
	indexed := make(map[seriesKey]bool, len(r.Operations))
	required := make(map[Operation]uint64)
	var requests, unexpected uint64
	for _, value := range r.Operations {
		if !value.Phase.valid() || !value.Surface.valid() || !value.Operation.valid() {
			fail("invalid operation/phase/origin label")
			continue
		}
		key := seriesKey{value.Phase, value.Surface, value.Operation}
		if indexed[key] {
			fail("duplicate operation/phase/origin report")
		}
		indexed[key] = true
		var outcomes, statuses uint64
		for _, count := range value.Outcomes {
			outcomes += count
		}
		for _, count := range value.Statuses {
			statuses += count
		}
		if outcomes != value.Count || statuses != value.Count {
			fail(string(value.Operation) + ": incomplete raw outcome/status accounting")
		}
		if value.Phase.baseline() && value.P95 > value.Operation.Limit() {
			fail(
				string(
					value.Phase,
				) + "/" + string(
					value.Surface,
				) + "/" + string(
					value.Operation,
				) + ": p95 target exceeded",
			)
		}
		phase := SustainedPhase
		if value.Operation == RegistrationWrite {
			phase = BurstPhase
		}
		if value.Phase == phase {
			required[value.Operation] += value.Count
		}
		if value.Phase.baseline() && value.Operation != DisplayUpdate &&
			value.Operation != DueJobStart {
			requests += value.Count
			unexpected += value.Outcomes[Unexpected]
		}
	}
	for _, operation := range []Operation{SessionRead, RegistrationRead, RegistrationWrite, MentorRead, MentorWrite, HelpRead, HelpClaim, HelpWrite, CueRead, CueWrite, ClarificationSearch, ShowcaseSearch, DisplayUpdate, DueJobStart} {
		if required[operation] < 30 {
			fail(string(operation) + ": fewer than30 observations in its actual baseline phase")
		}
	}
	if requests == 0 {
		fail("no measured baseline HTTP requests")
		return 0
	}
	rate := float64(unexpected) / float64(requests)
	if rate >= .01 {
		fail("unexpected baseline HTTP error rate is not below one percent")
	}
	return rate
}

func (r Report) validateOracles(fail func(string)) {
	oracles := make(map[string]Oracle, len(r.Oracles))
	for _, oracle := range r.Oracles {
		if _, exists := oracles[oracle.Name]; exists {
			fail("duplicate domain oracle")
		}
		oracles[oracle.Name] = oracle
	}
	for _, name := range requiredOracles {
		oracle, exists := oracles[name]
		if !exists || !oracle.Passed || oracle.Observations == 0 ||
			len(oracle.EvidenceSHA256) != 64 {
			fail(name + ": independent oracle missing or failed")
		}
	}
}
