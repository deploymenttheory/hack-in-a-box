package smtpintegration

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

func bodyWithCalendar(body string, calendar *maildelivery.Calendar) (string, error) {
	if calendar == nil {
		return body, nil
	}
	details, err := maildelivery.DateDetails(*calendar)
	if err != nil {
		return "", fmt.Errorf("render approved dates: %w", err)
	}
	return body + "\n\n" + details + "\n" + calendar.WorkspaceURL, nil
}

// ProjectSelfTest shares the delivery formatter without retaining sequence,
// preparing MIME, storing an intent, resolving an address or contacting SMTP.
func (s Service) ProjectSelfTest(
	ctx context.Context,
	q *dbgen.Queries,
	request communications.TestMessage,
) (communications.TestPresentation, error) {
	result := communications.TestPresentation{PlainText: request.PlainText}
	if !request.IncludeCalendar {
		return result, nil
	}
	calendar, err := s.calendarProjection(ctx, q, request.EventID, true)
	if err != nil {
		return result, err
	}
	result.PlainText, err = bodyWithCalendar(request.PlainText, calendar)
	if err != nil {
		return result, err
	}
	result.Calendar = &communications.TestCalendar{
		Start:        calendar.Start,
		End:          calendar.End,
		Timezone:     calendar.Timezone,
		Summary:      calendar.Summary,
		WorkspaceURL: calendar.WorkspaceURL,
	}
	return result, nil
}
