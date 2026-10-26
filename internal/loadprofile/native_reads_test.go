package loadprofile

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
)

// These are component rejection controls over synthetic projections. The real
// native-load/transaction/authentication proof requires the integrated runtime.
func TestNativeSearchCannotPassWithHiddenOrStaleAnswersAndEmptyFilteredPages(t *testing.T) {
	event, answer, project := uuid.New(), uuid.New(), uuid.New()
	binding := ReadBinding{
		Event:      event,
		Operations: []Operation{ClarificationSearch, ShowcaseSearch},
		Visibility: SearchVisibility{
			Needle:          "synthetic-common",
			MinimumAnswers:  1,
			MinimumProjects: 1,
			Answers:         map[uuid.UUID]int64{answer: 2},
			Projects:        map[uuid.UUID]int64{project: 3},
			Events:          map[uuid.UUID]string{event: "Synthetic visible event"},
			Forbidden:       []string{"PRIVATE_SYNTHETIC_CANARY"},
		},
	}
	validAnswer := clarification.PublishedAnswer{
		ID:            answer,
		Version:       2,
		Current:       true,
		SourceCurrent: true,
	}
	encode := func(value any) []byte {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if err := validatePublicSearch(
		encode(clarification.PublicList{Items: []clarification.PublishedAnswer{validAnswer}}),
		binding,
		ClarificationSearch,
	); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*clarification.PublishedAnswer){"other-answer": func(value *clarification.PublishedAnswer) { value.ID = uuid.New() }, "old-version": func(value *clarification.PublishedAnswer) { value.Version = 1 }, "old-source": func(value *clarification.PublishedAnswer) { value.SourceCurrent = false }, "private": func(value *clarification.PublishedAnswer) { value.Document.Body = "PRIVATE_SYNTHETIC_CANARY" }} {
		t.Run(name, func(t *testing.T) {
			value := validAnswer
			change(&value)
			err := validatePublicSearch(
				encode(clarification.PublicList{Items: []clarification.PublishedAnswer{value}}),
				binding,
				ClarificationSearch,
			)
			if err == nil {
				t.Fatal("hidden/stale common answer accepted")
			}
			if strings.Contains(err.Error(), "PRIVATE_SYNTHETIC_CANARY") {
				t.Fatal("private canary copied into evidence")
			}
		})
	}
	if err := validatePublicSearch(
		encode(clarification.PublicList{}),
		binding,
		ClarificationSearch,
	); err == nil {
		t.Fatal("empty post-permission page became successful search")
	}
	validProject := outcomes.ProjectPage{
		Items:  []outcomes.Project{{ID: project, Version: 3}},
		Events: []outcomes.ProjectEvent{{ID: event, Name: "Synthetic visible event"}},
	}
	if err := validatePublicSearch(encode(validProject), binding, ShowcaseSearch); err != nil {
		t.Fatal(err)
	}
	validProject.Events[0].ID = uuid.New()
	if err := validatePublicSearch(encode(validProject), binding, ShowcaseSearch); err == nil {
		t.Fatal("hidden event metadata escaped permission oracle")
	}
}

func TestNativeRegistrationReadCannotDiscloseOtherPersonOrEvent(t *testing.T) {
	event, person := uuid.New(), uuid.New()
	binding := ReadBinding{Event: event}
	for _, view := range []registration.View{{ID: uuid.New(), EventID: event, PersonID: uuid.New()}, {ID: uuid.New(), EventID: uuid.New(), PersonID: person}} {
		raw, err := json.Marshal(struct {
			Items []registration.View `json:"items"`
		}{[]registration.View{view}})
		if err != nil {
			t.Fatal(err)
		}
		if err := validateNativeRead(raw, person, binding, RegistrationRead); err == nil {
			t.Fatal("other native registration treated as actor activity")
		}
	}
	if err := decodeNative([]byte("{} {\"private\":\"extra\"}"), new(struct{})); err == nil {
		t.Fatal("trailing private JSON accepted")
	}
}

func TestNativeReaderCannotMutateItsIndependentPublicationAllowlist(t *testing.T) {
	event, id := uuid.New(), uuid.New()
	source := map[string]ReadBinding{
		"actor": {
			Event:      event,
			Operations: []Operation{ClarificationSearch},
			Visibility: SearchVisibility{Answers: map[uuid.UUID]int64{id: 1}},
		},
	}
	reader, err := NewNativeReads(source)
	if err != nil {
		t.Fatal(err)
	}
	source["actor"].Visibility.Answers[id] = 99
	source["actor"].Operations[0] = RegistrationWrite
	if reader.bindings["actor"].Visibility.Answers[id] != 1 ||
		reader.bindings["actor"].Operations[0] != ClarificationSearch {
		t.Fatal("response-side mutation changed independent workload oracle")
	}
}
