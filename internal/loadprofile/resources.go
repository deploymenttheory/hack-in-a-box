package loadprofile

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var errLoadResources = errors.New("invalid load resources")

type ResourceSample struct {
	At            time.Time `json:"at"`
	Service       string    `json:"service"`
	CPUPercent    string    `json:"cpu_percent"`
	MemoryUsage   string    `json:"memory_usage"`
	MemoryPercent string    `json:"memory_percent"`
	NetworkIO     string    `json:"network_io"`
	BlockIO       string    `json:"block_io"`
	PIDs          string    `json:"pids"`
}

// DockerResources reads only the explicitly supplied project container IDs.
// It never enumerates, modifies or prunes the user's other Docker resources.
// The caller retains the complete time series, including read failures.
func DockerResources(ctx context.Context, services map[string]string) ([]ResourceSample, error) {
	if len(services) == 0 {
		return nil, fmt.Errorf("%w: explicit scoped containers required", errLoadResources)
	}
	labels := make(map[string]string, len(services))
	args := []string{"stats", "--no-stream", "--format", "{{json .}}"}
	for service, id := range services {
		if service == "" || len(id) < 12 || len(id) > 64 ||
			strings.Trim(id, "0123456789abcdef") != "" {
			return nil, fmt.Errorf(
				"%w: resource sampling requires an exact scoped container ID",
				errLoadResources,
			)
		}
		if _, exists := labels[id]; exists {
			return nil, fmt.Errorf(
				"%w: one resource container cannot represent two services",
				errLoadResources,
			)
		}
		labels[id] = service
		args = append(args, id)
	}
	raw, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("%w: scoped Docker resource sample failed", errLoadResources)
	}
	return parseDockerResources(raw, labels, time.Now())
}

func parseDockerResources(
	raw []byte,
	labels map[string]string,
	at time.Time,
) ([]ResourceSample, error) {
	var samples []ResourceSample
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	seen := make(map[string]bool, len(labels))
	for scanner.Scan() {
		var row map[string]string
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			return nil, fmt.Errorf("%w: invalid Docker resource JSON", errLoadResources)
		}
		// Docker prints a shortened ID even when given the complete container ID.
		service := ""
		matched := ""
		for id, label := range labels {
			if len(row["ID"]) >= 12 && strings.HasPrefix(id, row["ID"]) {
				if service != "" {
					return nil, fmt.Errorf(
						"%w: ambiguous scoped Docker resource ID",
						errLoadResources,
					)
				}
				service, matched = label, id
			}
		}
		if service == "" || seen[matched] {
			return nil, fmt.Errorf(
				"%w: unexpected or duplicate Docker resource container",
				errLoadResources,
			)
		}
		seen[matched] = true
		samples = append(
			samples,
			ResourceSample{
				At:            at,
				Service:       service,
				CPUPercent:    row["CPUPerc"],
				MemoryUsage:   row["MemUsage"],
				MemoryPercent: row["MemPerc"],
				NetworkIO:     row["NetIO"],
				BlockIO:       row["BlockIO"],
				PIDs:          row["PIDs"],
			},
		)
	}
	if scanner.Err() != nil || len(samples) != len(labels) {
		return nil, fmt.Errorf("%w: incomplete scoped Docker resource sample", errLoadResources)
	}
	return samples, nil
}
