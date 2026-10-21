package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Store struct{ Queries *dbgen.Queries }

var _ scs.CtxStore = Store{}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s Store) Delete(token string) error { return s.DeleteCtx(context.Background(), token) }
func (s Store) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

func (s Store) Commit(token string, data []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, data, expiry)
}

func (s Store) DeleteCtx(ctx context.Context, token string) error {
	if err := s.Queries.DeleteSession(ctx, digest(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s Store) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	data, err := s.Queries.FindSession(ctx, digest(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find session: %w", err)
	}
	return data, true, nil
}

func (s Store) CommitCtx(ctx context.Context, token string, data []byte, expiry time.Time) error {
	if err := s.Queries.PutSession(
		ctx,
		dbgen.PutSessionParams{Token: digest(token), Data: data, Expiry: expiry},
	); err != nil {
		return fmt.Errorf("persist session: %w", err)
	}
	return nil
}
