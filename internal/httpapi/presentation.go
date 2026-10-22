package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
)

type displayTokenKey struct{}

// Only exact display GET routes use display capabilities. Nothing else can
// escape the ordinary person-session/CSRF boundary or finite response timeout.
func displayRoute(r *http.Request) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if r.Method != http.MethodGet || (len(parts) != 6 && len(parts) != 7) || parts[0] != "api" ||
		parts[1] != "v1" ||
		parts[2] != "events" ||
		parts[4] != "display" {
		return false
	}
	if len(parts) == 7 {
		if parts[5] != "assets" {
			return false
		}
		if _, err := uuid.Parse(parts[6]); err != nil {
			return false
		}
	} else if parts[5] != "snapshot" && parts[5] != "stream" {
		return false
	}

	_, err := uuid.Parse(parts[3])
	return err == nil
}

func displayToken(ctx context.Context) string {
	token, _ := ctx.Value(displayTokenKey{}).(string)
	return token
}

func displayAuthorization(r *http.Request) (string, error) {
	if r.URL.RawQuery != "" {
		return "", access.ErrInvalid
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || len(auth) < 39 || len(auth) > 207 {
		return "", access.ErrUnauthenticated
	}
	return strings.TrimPrefix(auth, "Bearer "), nil
}

func finiteOrDisplayTimeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		finite := timeout(duration)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if displayRoute(r) && strings.HasSuffix(r.URL.Path, "/stream") {
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
				defer cancel()
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			finite.ServeHTTP(w, r)
		})
	}
}

func (h handlers) GetPresentation(
	ctx context.Context,
	r contract.GetPresentationRequestObject,
) (contract.GetPresentationResponseObject, error) {
	p, err := h.Presentation.Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("presentation: %w", err)
	}
	return convertDTO[contract.GetPresentation200JSONResponse](p)
}

func (h handlers) ChangePresentation(
	ctx context.Context,
	r contract.ChangePresentationRequestObject,
) (contract.ChangePresentationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	c, err := convertDTO[presentation.Command](r.Body)
	if err != nil {
		return nil, err
	}
	p, err := h.Presentation.Change(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), c)
	if err != nil {
		return nil, fmt.Errorf("presentation update: %w", err)
	}
	return convertDTO[contract.ChangePresentation200JSONResponse](p)
}

func (h handlers) PairDisplay(
	ctx context.Context,
	r contract.PairDisplayRequestObject,
) (contract.PairDisplayResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[presentation.PairInput](r.Body)
	if err != nil {
		return nil, err
	}
	p, err := h.Presentation.Pair(ctx, actor(ctx).ID, r.EventId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("display pairing: %w", err)
	}
	return convertDTO[contract.PairDisplay201JSONResponse](p)
}

func (h handlers) RevokeDisplay(
	ctx context.Context,
	r contract.RevokeDisplayRequestObject,
) (contract.RevokeDisplayResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	p, err := h.Presentation.Revoke(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.PairingId,
		revision(r.Params.IfMatch),
		r.Body.Reason,
	)
	if err != nil {
		return nil, fmt.Errorf("display revocation: %w", err)
	}
	return convertDTO[contract.RevokeDisplay200JSONResponse](p)
}

func (h handlers) GetDisplaySnapshot(
	ctx context.Context,
	r contract.GetDisplaySnapshotRequestObject,
) (contract.GetDisplaySnapshotResponseObject, error) {
	p, err := h.Presentation.Snapshot(ctx, r.EventId, displayToken(ctx))
	if err != nil {
		return nil, fmt.Errorf("display view: %w", err)
	}
	return convertDTO[contract.GetDisplaySnapshot200JSONResponse](p)
}

func (h handlers) StreamDisplay(
	ctx context.Context,
	r contract.StreamDisplayRequestObject,
) (contract.StreamDisplayResponseObject, error) {
	initialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	p, err := h.Presentation.Snapshot(initialCtx, r.EventId, displayToken(ctx))
	if err != nil {
		return nil, fmt.Errorf("open display stream: %w", err)
	}
	return displayStream{
		ctx:     ctx,
		service: h.Presentation,
		eventID: r.EventId,
		token:   displayToken(ctx),
		initial: p,
	}, nil
}

type displayStream struct {
	ctx     context.Context
	service presentation.Service
	eventID uuid.UUID
	token   string
	initial presentation.Snapshot
}

func (s displayStream) VisitStreamDisplayResponse(w http.ResponseWriter) error {
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(event string, id int64, data any) error {
		body, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("encode display frame: %w", err)
		}
		if err = controller.SetWriteDeadline(
			time.Now().Add(5 * time.Second),
		); err != nil &&
			!errors.Is(err, http.ErrNotSupported) {
			return fmt.Errorf("bound display write: %w", err)
		}
		if _, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, event, body); err != nil {
			return fmt.Errorf("write display frame: %w", err)
		}
		if err = controller.Flush(); err != nil {
			return fmt.Errorf("flush display frame: %w", err)
		}
		return nil
	}
	// Every connection starts from the latest complete snapshot, never replaying
	// stale cues from Last-Event-ID or inheriting an account's permissions.
	if err := send("snapshot", s.initial.Sequence, s.initial); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			p, err := s.service.Snapshot(ctx, s.eventID, s.token)
			cancel()
			if err != nil {
				event := "reconnect"
				if errors.Is(err, access.ErrUnauthenticated) ||
					errors.Is(err, access.ErrForbidden) {
					event = "revoked"
				}
				return send(event, s.initial.Sequence, struct{}{})
			}
			if err = send("snapshot", p.Sequence, p); err != nil {
				return err
			}
		}
	}
}

func (h handlers) GetDisplayAsset(
	ctx context.Context,
	r contract.GetDisplayAssetRequestObject,
) (contract.GetDisplayAssetResponseObject, error) {
	a, err := h.Presentation.Asset(ctx, r.EventId, displayToken(ctx), r.AssetId)
	if err != nil {
		return nil, fmt.Errorf("display asset: %w", err)
	}
	return displayAsset{mediaType: a.MediaType, content: a.Content}, nil
}

type displayAsset struct {
	mediaType string
	content   []byte
}

func (a displayAsset) VisitGetDisplayAssetResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", a.mediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err := w.Write(a.content)
	if err != nil {
		return fmt.Errorf("write display asset: %w", err)
	}
	return nil
}
