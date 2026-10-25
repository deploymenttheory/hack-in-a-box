package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// lifecycleOIDC is a synthetic provider, not an application authentication
// shortcut. The application's normal discovery, state, PKCE, nonce, JWT, user
// creation, bootstrap-subject and independent-session paths remain in use.
// Authentication expiry deliberately uses wall time, not the rehearsal clock.
type lifecycleOIDC struct {
	issuer  string
	signer  jose.Signer
	public  jose.JSONWebKey
	clients map[string]string
	people  map[string]lifecyclePerson
	mu      sync.Mutex
	codes   map[string]lifecycleCode
}

type lifecyclePerson struct {
	Subject string `json:"subject"`
	Name    string `json:"name"`
	Email   string `json:"email"`
}

type lifecycleCode struct {
	ClientID, Redirect, Nonce, Challenge, Subject string
	Expires                                       time.Time
}

func newLifecycleOIDC(issuer string, clients map[string]string) (*lifecycleOIDC, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "synthetic-lifecycle"),
	)
	if err != nil {
		return nil, err
	}
	people := map[string]lifecyclePerson{}
	for _, role := range []string{"owner", "coordinator", "mentor-a", "mentor-b", "judge-a", "judge-b", "judge-c", "mc", "logistics", "stranger", "former-organiser", "early-access"} {
		people[role] = lifecyclePerson{Subject: role, Name: "Synthetic " + role, Email: role + "@example.test"}
	}
	for i := 1; i <= 40; i++ {
		subject := fmt.Sprintf("attendee-%02d", i)
		people[subject] = lifecyclePerson{Subject: subject, Name: fmt.Sprintf("Synthetic attendee %02d", i), Email: subject + "@example.test"}
	}
	return &lifecycleOIDC{
		issuer: issuer, signer: signer,
		public:  jose.JSONWebKey{Key: &key.PublicKey, KeyID: "synthetic-lifecycle", Algorithm: "RS256", Use: "sig"},
		clients: clients, people: people, codes: map[string]lifecycleCode{},
	}, nil
}

func (p *lifecycleOIDC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		lifecycleJSON(w, map[string]any{
			"issuer": p.issuer, "authorization_endpoint": p.issuer + "/authorize",
			"token_endpoint": p.issuer + "/token", "jwks_uri": p.issuer + "/keys",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"},
		})
	case "/keys":
		lifecycleJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{p.public}})
	case "/authorize":
		p.authorize(w, r)
	case "/token":
		p.token(w, r)
	default:
		http.NotFound(w, r)
	}
}

var lifecycleLogin = template.Must(template.New("synthetic-provider").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><title>Synthetic lifecycle identity</title><h1>Synthetic identity provider</h1><p>This provider belongs only to the isolated lifecycle Compose network.</p><form method="get" action="/authorize">{{range $key,$values := .Query}}{{range $values}}<input type="hidden" name="{{$key}}" value="{{.}}">{{end}}{{end}}<label>Synthetic actor <select name="actor">{{range .People}}<option value="{{.Subject}}">{{.Name}}</option>{{end}}</select></label><button type="submit">Continue through normal OIDC</button></form></html>`))

func (p *lifecycleOIDC) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect, exists := p.clients[q.Get("client_id")]
	if r.Method != http.MethodGet || !exists || q.Get("redirect_uri") != redirect ||
		q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" ||
		len(q.Get("code_challenge")) != 43 || len(q.Get("state")) < 20 || len(q.Get("nonce")) < 20 ||
		!strings.Contains(" "+q.Get("scope")+" ", " openid ") {
		http.Error(w, "invalid synthetic authorization request", http.StatusBadRequest)
		return
	}
	person, selected := p.people[q.Get("actor")]
	if !selected {
		if q.Get("actor") != "" {
			http.Error(w, "unknown synthetic identity", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = lifecycleLogin.Execute(w, struct {
			Query  url.Values
			People map[string]lifecyclePerson
		}{q, p.people})
		return
	}
	code := rand.Text()
	p.mu.Lock()
	for key, value := range p.codes {
		if !time.Now().Before(value.Expires) {
			delete(p.codes, key)
		}
	}
	p.codes[code] = lifecycleCode{
		ClientID: q.Get("client_id"), Redirect: redirect, Nonce: q.Get("nonce"),
		Challenge: q.Get("code_challenge"), Subject: person.Subject, Expires: time.Now().Add(2 * time.Minute),
	}
	p.mu.Unlock()
	target, _ := url.Parse(redirect)
	values := target.Query()
	values.Set("code", code)
	values.Set("state", q.Get("state"))
	target.RawQuery = values.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func (p *lifecycleOIDC) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid token form", http.StatusBadRequest)
		return
	}
	clientID, secret, basic := r.BasicAuth()
	if !basic {
		clientID, secret = r.Form.Get("client_id"), r.Form.Get("client_secret")
	}
	p.mu.Lock()
	code, exists := p.codes[r.Form.Get("code")]
	delete(p.codes, r.Form.Get("code"))
	p.mu.Unlock()
	digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	if !exists || !time.Now().Before(code.Expires) || clientID != code.ClientID || secret != "synthetic-lifecycle-secret" ||
		r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("redirect_uri") != code.Redirect ||
		base64.RawURLEncoding.EncodeToString(digest[:]) != code.Challenge {
		http.Error(w, "invalid or consumed synthetic authorization code", http.StatusBadRequest)
		return
	}
	person := p.people[code.Subject]
	claims, err := json.Marshal(map[string]any{
		"iss": p.issuer, "sub": person.Subject, "aud": code.ClientID, "nonce": code.Nonce,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
		"name": person.Name, "email": person.Email, "email_verified": true,
	})
	if err != nil {
		http.Error(w, "synthetic signing failed", http.StatusInternalServerError)
		return
	}
	signed, err := p.signer.Sign(claims)
	if err != nil {
		http.Error(w, "synthetic signing failed", http.StatusInternalServerError)
		return
	}
	compact, err := signed.CompactSerialize()
	if err != nil {
		http.Error(w, "synthetic signing failed", http.StatusInternalServerError)
		return
	}
	lifecycleJSON(w, map[string]any{"access_token": "synthetic-provider-only", "token_type": "Bearer", "id_token": compact, "expires_in": 120})
}

func lifecycleJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(value)
}
