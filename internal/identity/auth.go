package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
)

var (
	ErrLogin = errors.New("sign-in could not be completed; start sign-in again")
	ErrCSRF  = errors.New("invalid CSRF token or origin")
)

type Settings struct {
	Issuer, ClientID, ClientSecret, Origin, BootstrapSubject string
	CookiePrefix                                             string
	Secure                                                   bool
}
type Auth struct {
	Pool     *pgxpool.Pool
	Sessions *scs.SessionManager
	settings Settings
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

func New(ctx context.Context, pool *pgxpool.Pool, settings Settings) (*Auth, error) {
	if settings.CookiePrefix == "" {
		settings.CookiePrefix = "hackbox"
	}
	sessions := scs.New()
	sessions.Store = Store{dbgen.New(pool)}
	sessions.Lifetime = 12 * time.Hour
	sessions.IdleTimeout = 30 * time.Minute
	sessions.Cookie.Name = settings.CookiePrefix + "_session"
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.Secure = settings.Secure
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Persist = false
	auth := &Auth{Pool: pool, Sessions: sessions, settings: settings}
	if settings.Issuer == "" {
		return auth, nil
	}
	client := &http.Client{Timeout: 10 * time.Second}
	ctx = oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(ctx, settings.Issuer)
	if err != nil {
		return nil, ErrLogin
	}
	auth.verifier = provider.Verifier(&oidc.Config{ClientID: settings.ClientID})
	auth.oauth = oauth2.Config{
		ClientID:     settings.ClientID,
		ClientSecret: settings.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  settings.Origin + "/auth/callback",
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return auth, nil
}

func (a *Auth) Login(w http.ResponseWriter, r *http.Request) error {
	if a.verifier == nil {
		return ErrLogin
	}
	state, nonce, browser := rand.Text(), rand.Text(), rand.Text()
	verifier := oauth2.GenerateVerifier()
	err := dbgen.New(a.Pool).
		PutLoginAttempt(r.Context(), dbgen.PutLoginAttemptParams{StateHash: digest(state), BrowserHash: digest(browser), Nonce: nonce, Verifier: verifier, ExpiresAt: time.Now().Add(5 * time.Minute), ReturnTo: SafeReturnTo(r.URL.Query().Get("return_to"))})
	if err != nil {
		return ErrLogin
	}
	http.SetCookie(
		w,
		&http.Cookie{ // #nosec G124 -- Secure is required by production configuration; local HTTP fixtures use false.
			Name:     a.settings.CookiePrefix + "_login",
			Value:    browser,
			Path:     "/auth/callback",
			HttpOnly: true,
			Secure:   a.settings.Secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300,
		},
	)
	http.Redirect(
		w,
		r,
		a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)),
		http.StatusFound,
	)
	return nil
}

func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) error {
	if a.verifier == nil {
		return ErrLogin
	}
	cookie, err := r.Cookie(a.settings.CookiePrefix + "_login")
	if err != nil || cookie.Value == "" || r.URL.Query().Get("state") == "" {
		return ErrLogin
	}
	http.SetCookie(
		w,
		&http.Cookie{ // #nosec G124 -- Deletes the cookie with the validated deployment attributes.
			Name:     a.settings.CookiePrefix + "_login",
			Path:     "/auth/callback",
			HttpOnly: true,
			Secure:   a.settings.Secure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		},
	)
	attempt, err := dbgen.New(a.Pool).
		ConsumeLoginAttempt(r.Context(), dbgen.ConsumeLoginAttemptParams{StateHash: digest(r.URL.Query().Get("state")), BrowserHash: digest(cookie.Value)})
	if err != nil || r.URL.Query().Get("code") == "" {
		return ErrLogin
	}
	ctx := oidc.ClientContext(r.Context(), &http.Client{Timeout: 10 * time.Second})
	token, err := a.oauth.Exchange(
		ctx,
		r.URL.Query().Get("code"),
		oauth2.VerifierOption(attempt.Verifier),
	)
	if err != nil {
		return ErrLogin
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return ErrLogin
	}
	claims, err := a.verifier.Verify(ctx, raw)
	if err != nil || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(attempt.Nonce)) != 1 ||
		claims.Subject == "" {
		return ErrLogin
	}
	var profile struct {
		Name            string `json:"name"`
		Email           string `json:"email"`
		EmailVerified   bool   `json:"email_verified"`
		AuthorizedParty string `json:"azp"`
	}
	if err := claims.Claims(&profile); err != nil {
		return ErrLogin
	}
	if (len(claims.Audience) > 1 || profile.AuthorizedParty != "") &&
		profile.AuthorizedParty != a.settings.ClientID {
		return ErrLogin
	}
	if len(profile.Name) > 160 {
		profile.Name = ""
	}
	var person dbgen.Person
	err = database.Within(ctx, a.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		var err error
		person, err = q.UpsertIdentity(
			ctx,
			dbgen.UpsertIdentityParams{
				Issuer:      claims.Issuer,
				Subject:     claims.Subject,
				DisplayName: profile.Name,
			},
		)
		if err != nil {
			return fmt.Errorf("upsert identity: %w", err)
		}
		if !person.Active {
			return access.ErrUnauthenticated
		}
		if err := mailcontact.SyncClaimInTransaction(
			ctx,
			q,
			person.ID,
			profile.Email,
			profile.EmailVerified,
		); err != nil {
			return fmt.Errorf("sync private email contact: %w", err)
		}
		if claims.Subject == a.settings.BootstrapSubject && a.settings.BootstrapSubject != "" {
			changed, err := q.BootstrapAdmin(ctx, person.ID)
			if err != nil {
				return fmt.Errorf("bootstrap admin: %w", err)
			}
			if changed > 0 {
				if err := audit.Append(
					ctx,
					q,
					audit.Record{
						ActorID:  &person.ID,
						TargetID: person.ID,
						Action:   audit.AdminBootstrapped,
						Reason:   audit.Bootstrap,
					},
				); err != nil {
					return fmt.Errorf("append: %w", err)
				}
			}
			person, err = q.Person(ctx, person.ID)
			if err != nil {
				return fmt.Errorf("person: %w", err)
			}
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &person.ID,
				TargetID: person.ID,
				Action:   audit.SignedIn,
				Reason:   audit.Authentication,
			},
		)
	})
	if err != nil {
		return ErrLogin
	}
	if err := a.Sessions.RenewToken(ctx); err != nil {
		return ErrLogin
	}
	a.Sessions.Put(ctx, "person_id", person.ID.String())
	a.Sessions.Put(ctx, "session_revision", person.SessionRevision)
	a.Sessions.Put(ctx, "csrf", rand.Text()+rand.Text())
	http.Redirect(w, r, SafeReturnTo(attempt.ReturnTo), http.StatusSeeOther)
	return nil
}

// SafeReturnTo accepts only local application pages, including on callback.
// It rejects encoded slashes/backslashes and auth/API destinations as well as
// external URLs, so a login cannot become an open redirect or trigger an action.
func SafeReturnTo(raw string) string {
	if raw == "" || len(raw) > 2048 || !strings.HasPrefix(raw, "/") ||
		strings.ContainsAny(raw, "\\\r\n\x00") {
		return "/"
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.IsAbs() || u.Host != "" || strings.HasPrefix(u.Path, "//") ||
		strings.ContainsAny(u.Path, "\\\r\n\x00") {
		return "/"
	}
	for _, prefix := range []string{"/auth", "/api", "/ops"} {
		if u.Path == prefix || strings.HasPrefix(u.Path, prefix+"/") {
			return "/"
		}
	}
	return u.RequestURI()
}

func (a *Auth) Current(ctx context.Context) (dbgen.Person, error) {
	id, err := uuid.Parse(a.Sessions.GetString(ctx, "person_id"))
	if err != nil {
		return dbgen.Person{}, access.ErrUnauthenticated
	}
	person, err := dbgen.New(a.Pool).Person(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Person{}, access.ErrUnauthenticated
	}
	if err != nil {
		return dbgen.Person{}, fmt.Errorf("load session identity: %w", err)
	}
	if !person.Active || person.SessionRevision != a.Sessions.GetInt64(ctx, "session_revision") {
		return dbgen.Person{}, access.ErrUnauthenticated
	}
	return person, nil
}

func (a *Auth) CSRF(r *http.Request) error {
	expected := a.Sessions.GetString(r.Context(), "csrf")
	if r.Header.Get("Origin") != a.settings.Origin || expected == "" ||
		subtle.ConstantTimeCompare([]byte(expected), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
		return ErrCSRF
	}
	return nil
}

func (a *Auth) Logout(ctx context.Context) error {
	if err := a.Sessions.Destroy(ctx); err != nil {
		return fmt.Errorf("destroy session: %w", err)
	}
	return nil
}
