package loadprofile

type Phase string

const (
	SetupPhase      Phase = "setup"
	WarmupPhase     Phase = "warmup"
	BurstPhase      Phase = "burst"
	SaturationPhase Phase = "saturation"
	SustainedPhase  Phase = "sustained"
	ProbePhase      Phase = "probe"
)

var phases = [...]Phase{
	SetupPhase,
	WarmupPhase,
	BurstPhase,
	SaturationPhase,
	SustainedPhase,
	ProbePhase,
}

func (p Phase) valid() bool {
	for _, phase := range phases {
		if p == phase {
			return true
		}
	}
	return false
}
func (p Phase) baseline() bool { return p == BurstPhase || p == SustainedPhase }

type Surface string

const (
	PublicSurface  Surface = "public"
	AdminSurface   Surface = "admin"
	DisplaySurface Surface = "display"
	WorkerSurface  Surface = "worker"
)

var surfaces = [...]Surface{PublicSurface, AdminSurface, DisplaySurface, WorkerSurface}

func (s Surface) valid() bool {
	for _, surface := range surfaces {
		if s == surface {
			return true
		}
	}
	return false
}

type seriesKey struct {
	phase     Phase
	surface   Surface
	operation Operation
}

func nativeSample(actor Actor, operation Operation, response Response) Measurement {
	return Measurement{
		Phase:     actor.Phase,
		Surface:   actor.Session.surface,
		Operation: operation,
		Duration:  response.Duration,
		Status:    response.Status,
		Outcome:   Unexpected,
	}
}
