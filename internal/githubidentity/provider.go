// Package githubidentity verifies GitHub ownership independently of organisation SSO.
package githubidentity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-github/v89/github"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

var (
	ErrLink     = errors.New("GitHub verification failed; start linking again")
	ErrDisabled = errors.New("GitHub linking is not configured")
)

type (
	Settings struct{ ClientID, ClientSecret, TokenKey, Origin string }
	Service  struct {
		Pool   *pgxpool.Pool
		oauth  oauth2.Config
		seal   cipher.AEAD
		client *http.Client
		apiURL string
	}
)

func New(pool *pgxpool.Pool, c Settings) (*Service, error) {
	s := &Service{
		Pool: pool,
		client: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
		apiURL: "https://api.github.com/",
	}
	if c.ClientID == "" {
		return s, nil
	}
	key, err := base64.StdEncoding.DecodeString(c.TokenKey)
	if err != nil || len(key) != 32 {
		return nil, ErrDisabled
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrDisabled
	}
	s.seal, err = cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, ErrDisabled
	}
	s.oauth = oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.Origin + "/auth/github/callback",
		Endpoint: oauth2.Endpoint{ // #nosec G101 -- Public GitHub OAuth endpoint URLs, not credentials.
			AuthURL:   "https://github.com/login/oauth/authorize",
			TokenURL:  "https://github.com/login/oauth/access_token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	return s, nil
}
func (s *Service) Enabled() bool { return s != nil && s.seal != nil }
func (s *Service) encrypt(person uuid.UUID, plain []byte) []byte {
	return s.seal.Seal(
		nil,
		nil,
		plain,
		person[:],
	) // #nosec G407 -- NewGCMWithRandomNonce generates and prepends a fresh nonce; its API requires nil here.
}

func (s *Service) decrypt(person uuid.UUID, value []byte) ([]byte, error) {
	p, err := s.seal.Open(nil, nil, value, person[:])
	if err != nil {
		return nil, ErrLink
	}
	return p, nil
}

type verified struct {
	ID    int64
	Login string
	Token []byte
}

func (s *Service) verify(
	ctx context.Context,
	actor uuid.UUID,
	code, verifier string,
) (verified, error) {
	var result verified
	token, err := s.oauth.Exchange(
		context.WithValue(ctx, oauth2.HTTPClient, s.client),
		code,
		oauth2.VerifierOption(verifier),
	)
	if err != nil || token.AccessToken == "" {
		return result, ErrLink
	}
	client, err := github.NewClient(
		github.WithHTTPClient(s.client),
		github.WithAuthToken(token.AccessToken),
		github.WithURLs(&s.apiURL, nil),
	)
	if err != nil {
		return result, ErrLink
	}
	user, _, err := client.Users.Get(ctx, "")
	if err != nil || user.GetID() <= 0 || user.GetLogin() == "" || user.GetType() != "User" {
		return result, ErrLink
	}
	// Do not retain the provider's arbitrary extra response fields or return tokens to callers.
	data, err := json.Marshal(struct {
		Access  string    `json:"access"`
		Refresh string    `json:"refresh"`
		Expiry  time.Time `json:"expiry"`
	}{token.AccessToken, token.RefreshToken, token.Expiry})
	if err != nil {
		return result, fmt.Errorf("encode protected credentials: %w", err)
	}
	return verified{ID: user.GetID(), Login: user.GetLogin(), Token: s.encrypt(actor, data)}, nil
}
