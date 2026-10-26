package loadprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
)

var setupProjectPattern = regexp.MustCompile(`^hackbox-load123-[a-z0-9][a-z0-9-]{0,63}$`)

type SetupClockStep struct {
	Container        string        `json:"owned_control_container"`
	Expected         time.Time     `json:"expected_domain_time"`
	Next             time.Time     `json:"next_domain_time"`
	Started          time.Time     `json:"actual_wall_started"`
	Elapsed          time.Duration `json:"actual_elapsed_ns"`
	OriginalComplete bool          `json:"original_control_response_complete"`
	OutputSHA256     string        `json:"control_output_sha256"`
}

// DockerSetupClock can run only the source-pinned private fixture controller,
// without network access, against this named Compose project's private volume.
// This is a SETUP control, never an app route or a runtime clock option.
type DockerSetupClock struct {
	Project string
	Image   string
	Source  string
	Steps   []SetupClockStep
}

func (c *DockerSetupClock) validate(ctx context.Context) error {
	if c == nil || !setupProjectPattern.MatchString(c.Project) ||
		!strings.HasPrefix(c.Image, "sha256:") ||
		len(c.Image) != 71 ||
		c.Image != strings.ToLower(c.Image) ||
		len(c.Source) != 40 {
		return fmt.Errorf("%w: exact private setup project/source/image required", errCohort)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(c.Image, "sha256:")); err != nil {
		return fmt.Errorf("%w: immutable setup image required", errCohort)
	}
	if _, err := hex.DecodeString(c.Source); err != nil {
		return fmt.Errorf("%w: exact setup source required", errCohort)
	}
	volumeArgs := []string{
		"volume",
		"inspect",
		"--format",
		"{{json .Labels}}",
		c.Project + "_fixture_private",
	}
	raw, err := exec.CommandContext(ctx, "docker", volumeArgs...).Output()
	if err != nil {
		return fmt.Errorf("inspect existing private clock volume: %w", err)
	}
	var labels map[string]string
	if json.Unmarshal(raw, &labels) != nil || labels["com.docker.compose.project"] != c.Project ||
		labels["com.docker.compose.volume"] != "fixture_private" {
		return fmt.Errorf(
			"%w: clock volume does not belong to this exact fixture project",
			errCohort,
		)
	}
	imageArgs := []string{"image", "inspect", "--format", "{{json .Config.Labels}}", c.Image}
	raw, err = exec.CommandContext(ctx, "docker", imageArgs...).Output()
	if err != nil {
		return fmt.Errorf("inspect immutable clock-controller image: %w", err)
	}
	var imageLabels map[string]string
	if json.Unmarshal(raw, &imageLabels) != nil ||
		imageLabels["org.opencontainers.image.revision"] != c.Source {
		return fmt.Errorf("%w: clock-controller source differs from pinned image", errCohort)
	}
	return nil
}

func (c *DockerSetupClock) Advance(ctx context.Context, expected, next time.Time) error {
	if !next.After(expected) {
		return fmt.Errorf("%w: strictly forward private setup phase required", errCohort)
	}
	if err := c.validate(ctx); err != nil {
		return err
	}
	step := SetupClockStep{
		Expected:  expected,
		Next:      next,
		Started:   time.Now(),
		Container: fmt.Sprintf("%s-clock-%d", c.Project, next.UnixNano()),
	}
	args := []string{
		"run",
		"--name", step.Container,
		"--label", "com.docker.compose.project=" + c.Project,
		"--label", "com.hackbox.load123.role=setup-clock-controller",
		"--rm",
		"--pull=never",
		"--network=none",
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--cpus=0.125",
		"--memory=64m",
		"--pids-limit=32",
		"--user=10001:10001", "--env=GOMAXPROCS=1",
		"--mount",
		"type=volume,src=" + c.Project + "_fixture_private,dst=/fixture",
		"--entrypoint=/app/loadfixture",
		c.Image,
		"-advance-setup-directory",
		"/fixture",
		"-expected-time",
		expected.Format(time.RFC3339Nano),
		"-next-time",
		next.Format(time.RFC3339Nano),
	}
	raw, err := exec.CommandContext(ctx, "docker", args...).Output()
	step.Elapsed = time.Since(step.Started)
	step.OriginalComplete = err == nil
	digest := sha256.Sum256(raw)
	step.OutputSHA256 = hex.EncodeToString(digest[:])
	c.Steps = append(c.Steps, step)
	if err != nil {
		return fmt.Errorf(
			"private clock control original response failed; do not retry without inspecting retained phase: %w",
			err,
		)
	}
	return nil
}

type WorkerLeaseObservation struct {
	Observed time.Time `json:"observed_wall_time"`
	Elected  time.Time `json:"river_elected_at"`
	Expires  time.Time `json:"river_lease_expires_at"`
}

// WorkerRunning observes River's real live leader lease, not a fabricated job
// or a direct handler invocation. Only the actual worker starts this client in
// the isolated fixture; API construction alone does not start River.
func (n NativeDatabase) WorkerRunning(ctx context.Context) (WorkerLeaseObservation, bool, error) {
	var observation WorkerLeaseObservation
	err := n.pool.QueryRow(ctx, `SELECT clock_timestamp(),elected_at,expires_at FROM river_leader WHERE expires_at>clock_timestamp() ORDER BY elected_at LIMIT 1`).
		Scan(&observation.Observed, &observation.Elected, &observation.Expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return observation, false, nil
	}
	if err != nil {
		return observation, false, fmt.Errorf("observe native River worker lease: %w", err)
	}
	return observation, true, nil
}

// CaptureStarts observes every actual native capture operation for this event,
// including missing/unstarted attempts. It never invokes a worker or scheduler.
func (n NativeDatabase) CaptureStarts(
	ctx context.Context,
	eventID uuid.UUID,
) ([]JobStartFact, error) {
	if err := n.scoped([]uuid.UUID{eventID}); err != nil {
		return nil, err
	}
	rows, err := n.pool.Query(
		ctx,
		`SELECT id FROM operations WHERE event_id=$1 AND kind=$2 ORDER BY id`,
		eventID,
		submissionfreeze.HandlerKind,
	)
	if err != nil {
		return nil, fmt.Errorf("observe native capture operation identities: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("decode native capture identity: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read complete native capture identities: %w", err)
	}
	if len(ids) == 0 {
		return []JobStartFact{}, nil
	}
	return n.JobStarts(ctx, ids)
}
