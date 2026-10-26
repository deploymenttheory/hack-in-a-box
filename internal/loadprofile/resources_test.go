package loadprofile

import (
	"testing"
	"time"
)

func TestResourceSamplingCannotSilentlySubstituteOrOmitAService(t *testing.T) {
	labels := map[string]string{
		"aaaaaaaaaaaa111111111111": "public",
		"bbbbbbbbbbbb222222222222": "worker",
	}
	valid := []byte(
		"{\"ID\":\"aaaaaaaaaaaa\",\"CPUPerc\":\"85.5%\",\"MemUsage\":\"120MiB / 256MiB\"}\n{\"ID\":\"bbbbbbbbbbbb\",\"CPUPerc\":\"99.0%\"}\n",
	)
	rows, err := parseDockerResources(valid, labels, time.Now())
	if err != nil || len(rows) != 2 || rows[0].Service != "public" ||
		rows[1].CPUPercent != "99.0%" {
		t.Fatal(rows, err)
	}
	for _, raw := range [][]byte{valid[:len(valid)/2], []byte("{\"ID\":\"aaaaaaaaaaaa\"}\n"), []byte("{\"ID\":\"cccccccccccc\"}\n"), []byte("{\"ID\":\"aaaaaaaaaaaa\"}\n{\"ID\":\"aaaaaaaaaaaa\"}\n")} {
		if _, err := parseDockerResources(raw, labels, time.Now()); err == nil {
			t.Fatal("incomplete or wrong resource sample accepted")
		}
	}
}
