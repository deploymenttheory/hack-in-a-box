package archives

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/preparation"
)

func (s Summary) CSV() ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.UseCRLF = true
	rows := make([][]string, 0, 1+len(s.Metrics)+len(s.Sources))
	rows = append(
		rows,
		[]string{
			"event_id",
			"event_name",
			"event_revision",
			"observed_at",
			"metric",
			"value",
			"unit",
			"denominator",
			"denominator_source",
			"source",
			"definition",
			"record_type",
			"availability",
			"unavailable_reason",
		},
	)
	for _, m := range s.Metrics {
		denominator := ""
		if m.Denominator != nil {
			denominator = strconv.FormatInt(*m.Denominator, 10)
		}
		rows = append(
			rows,
			[]string{
				s.EventID.String(),
				s.EventName,
				strconv.FormatInt(s.EventRevision, 10),
				s.ObservedAt.Format(time.RFC3339Nano),
				m.Key,
				strconv.FormatInt(m.Value, 10),
				m.Unit,
				denominator,
				m.DenominatorSource,
				m.Source,
				m.Definition,
				"metric",
				"",
				"",
			},
		)
	}
	for _, source := range s.Sources {
		rows = append(
			rows,
			[]string{
				s.EventID.String(),
				s.EventName,
				strconv.FormatInt(s.EventRevision, 10),
				s.ObservedAt.Format(time.RFC3339Nano),
				"",
				"",
				"",
				"",
				"",
				source.Source,
				"",
				"source_availability",
				source.State,
				source.Reason,
			},
		)
	}
	for _, row := range rows {
		for i := range row {
			row[i] = preparation.CSVCell(row[i])
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("encode archive CSV: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush archive CSV: %w", err)
	}
	return buffer.Bytes(), nil
}
