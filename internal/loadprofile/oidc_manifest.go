package loadprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const MaximumOIDCManifestBytes = 2 << 20

func ParseOIDCFixtureManifest(raw []byte) (OIDCFixtureManifest, error) {
	var manifest OIDCFixtureManifest
	if len(raw) == 0 || len(raw) > MaximumOIDCManifestBytes {
		return manifest, fmt.Errorf("%w: bounded explicit issuer manifest required", errCohort)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(new(any)) != io.EOF {
		return manifest, fmt.Errorf("%w: issuer manifest must be one exact JSON object", errCohort)
	}
	return manifest, manifest.Validate()
}

type OIDCFixturePerson struct {
	Subject string `json:"subject"`
	Name    string `json:"name"`
	Email   string `json:"email"`
}

// OIDCFixtureManifest configures only the synthetic EXTERNAL issuer. It cannot
// seed application people/sessions or grant event/resource authority. Release
// bootstrap uses one declared cohort staff member, not an extra dormant actor.
type OIDCFixtureManifest struct {
	Profile string                       `json:"profile"`
	Issuer  string                       `json:"issuer"`
	Clients map[string]string            `json:"clients"`
	People  map[string]OIDCFixturePerson `json:"people"`
}

func (m OIDCFixtureManifest) Validate() error {
	if m.Profile != "release" && m.Profile != "smoke" {
		return fmt.Errorf("%w: explicit issuer profile required", errCohort)
	}
	count := len(m.People)
	if count < 1 || count > 1000 || (m.Profile == "release" && count != 1000) {
		return fmt.Errorf("%w: explicit bounded release cohort required", errCohort)
	}
	if len(m.Clients) < 1 || len(m.Clients) > 2 {
		return fmt.Errorf("%w: one or two known callback clients required", errCohort)
	}
	if err := validateSignInOrigins(PublicSurface, m.Issuer, m.Issuer); err != nil {
		return err
	}
	for client, raw := range m.Clients {
		target, err := url.Parse(raw)
		if client == "" || err != nil || target.Scheme != "https" || target.Host == "" ||
			target.Path != "/auth/callback" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
			return fmt.Errorf("%w: exact HTTPS application callbacks required", errCohort)
		}
	}
	mailboxes := make(map[string]bool, len(m.People))
	for subject, person := range m.People {
		mailbox := strings.ToLower(strings.TrimSpace(person.Email))
		if subject == "" || person.Subject != subject || strings.TrimSpace(person.Name) == "" ||
			mailbox == "" || mailboxes[mailbox] {
			return fmt.Errorf("%w: distinct explicit subjects and mailboxes required", errCohort)
		}
		mailboxes[mailbox] = true
	}
	return nil
}

func OIDCManifestForCohort(
	plan CohortPlan,
	issuer string,
	clients map[string]string,
) (OIDCFixtureManifest, error) {
	manifest := OIDCFixtureManifest{
		Profile: plan.Profile,
		Issuer:  issuer,
		Clients: make(
			map[string]string,
			len(clients),
		),
		People: make(map[string]OIDCFixturePerson, len(plan.Members)),
	}
	if err := plan.Validate(); err != nil {
		return manifest, err
	}
	for key, value := range clients {
		manifest.Clients[key] = value
	}
	for _, member := range plan.Members {
		manifest.People[member.Subject] = OIDCFixturePerson{
			Subject: member.Subject,
			Name:    "Synthetic capacity " + member.Subject,
			Email:   member.Subject + "@example.test",
		}
	}
	return manifest, manifest.Validate()
}
