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
	Events                int    `json:"events"`
	CapacityPerEvent      int    `json:"capacity_per_event"`
	ConfirmedPerEvent     []int  `json:"confirmed_per_event"`
	AuthenticatedSessions int    `json:"authenticated_sessions"`
	Mentors               int    `json:"mentors"`
	DisplayStreams        int    `json:"display_streams"`
	Seed                  uint64 `json:"seed"`
}

type Oracle struct {
	Name           string `json:"name"`
	Passed         bool   `json:"passed"`
	Observations   uint64 `json:"observations"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

var RequiredOracles = []string{
	"registration-capacity-fifo",
	"accepted-write-reconciliation",
	"current-permission-private-search",
	"single-help-claim-owner",
	"scheduled-delivery-deduplication",
	"approved-followup-current-authority",
	"sse-current-source",
}

type Report struct {
	Source          string            `json:"source"`
	Tree            string            `json:"tree"`
	Images          map[string]string `json:"images"`
	Schema          int               `json:"schema"`
	Profile         string            `json:"profile"`
	Started         time.Time         `json:"started"`
	Finished        time.Time         `json:"finished"`
	Elapsed         time.Duration     `json:"monotonic_elapsed_ns"`
	Host            Host              `json:"host"`
	Dataset         Dataset           `json:"dataset"`
	Activity        ActivityReport    `json:"activity"`
	Operations      []OperationReport `json:"operations"`
	Integrity       IntegrityReport   `json:"integrity"`
	Oracles         []Oracle          `json:"oracles"`
	ResourceSamples int               `json:"resource_samples"`
	QueueSamples    int               `json:"queue_samples"`
	BurstCompleted  bool              `json:"burst_completed"`
	ExternalTLS     bool              `json:"external_tls"`
	DurableVolumes  bool              `json:"durable_volumes"`
	WorkerWallClock bool              `json:"worker_wall_clock"`
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
		r.Dataset.CapacityPerEvent == 200 &&
		r.Activity.ActiveEveryMinute >= 1000 &&
		r.Activity.Windows >= 30 &&
		r.BurstCompleted
	e.FullLoad = shape
	if !shape {
		fail(
			"complete five-event/1000-active-session burst and real 30-minute sustained profile not proven",
		)
	}
	r.validateBaseline(fail)
	r.validateDataset(fail)
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
	if len(r.Dataset.ConfirmedPerEvent) != 5 {
		fail("five-event committed admission census missing")
	} else {
		for _, confirmed := range r.Dataset.ConfirmedPerEvent {
			if confirmed != 200 {
				fail("full capacity of 200 confirmed competitors per event not exercised")
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
	if r.Integrity.Accepted == 0 || r.Integrity.Accepted != r.Integrity.Committed ||
		r.Integrity.Missing != 0 ||
		r.Integrity.Changed != 0 ||
		r.Integrity.DuplicateEffects != 0 {
		fail("accepted native write reconciliation failed")
	}
}

func (r Report) evaluateOperations(fail func(string)) float64 {
	var unexpectedRate float64
	indexed := make(map[Operation]OperationReport, len(r.Operations))
	var requests, unexpected uint64
	for _, operation := range r.Operations {
		if _, exists := indexed[operation.Operation]; exists {
			fail("duplicate operation report")
		}
		indexed[operation.Operation] = operation
		if operation.Operation != DisplayUpdate && operation.Operation != DueJobStart {
			requests += operation.Count
			unexpected += operation.Outcomes[Unexpected]
		}
	}
	if requests == 0 {
		fail("no measured HTTP requests")
	} else {
		unexpectedRate = float64(unexpected) / float64(requests)
		if unexpectedRate >= .01 {
			fail("unexpected HTTP error rate is not below one percent")
		}
	}
	for _, operation := range []Operation{SessionRead, RegistrationRead, RegistrationWrite, MentorWrite, HelpRead, HelpClaim, ClarificationSearch, ShowcaseSearch, DisplayUpdate, DueJobStart} {
		value, exists := indexed[operation]
		if !exists || value.Count < 30 {
			fail(string(operation) + ": fewer than 30 measured observations")
			continue
		}
		if value.P95 > operation.Limit() {
			fail(string(operation) + ": p95 target exceeded")
		}
		var outcomes uint64
		for _, count := range value.Outcomes {
			outcomes += count
		}
		if outcomes != value.Count {
			fail(string(operation) + ": incomplete outcome accounting")
		}
	}
	return unexpectedRate
}

func (r Report) validateOracles(fail func(string)) {
	oracles := make(map[string]Oracle, len(r.Oracles))
	for _, oracle := range r.Oracles {
		if _, exists := oracles[oracle.Name]; exists {
			fail("duplicate domain oracle")
		}
		oracles[oracle.Name] = oracle
	}
	for _, name := range RequiredOracles {
		oracle, exists := oracles[name]
		if !exists || !oracle.Passed || oracle.Observations == 0 ||
			len(oracle.EvidenceSHA256) != 64 {
			fail(name + ": independent oracle missing or failed")
		}
	}
}
