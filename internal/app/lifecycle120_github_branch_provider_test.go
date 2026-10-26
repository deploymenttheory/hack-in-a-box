package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// A synthetic external work transaction, not a Git object/history assertion or
// any native competition fact. The actual owner must explicitly review cutoff
// provenance; a later current-ref observation cannot prove the earlier head.
type lifecycleExternalWorkReceipt struct {
	ID            uuid.UUID `json:"id"`
	RepositoryID  int64     `json:"repository_id"`
	Name          string    `json:"name"`
	ActorID       int64     `json:"actor_id"`
	ParentSHA     string    `json:"parent_sha"`
	SHA           string    `json:"sha"`
	ContentSHA256 string    `json:"content_sha256"`
	CreatedAt     time.Time `json:"created_at"`
}

func (g *lifecycleGitHub) branchHTTP(w http.ResponseWriter, r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/github/repositories/") {
		return g.writeExternalWork(w, r)
	}
	prefix := "/github-api/repos/" + lifecycleGitHubOrganization + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) < 2 || parts[1] != "git" {
		return false
	}
	if r.Method != http.MethodGet || len(parts) != 5 || parts[2] != "ref" || parts[3] != "heads" || parts[4] != "main" || !lifecycleRepositoryName.MatchString(parts[0]) {
		w.WriteHeader(http.StatusNotFound)
		return true
	}
	name := parts[0]
	if !g.repositoryToken(r, name, "metadata", "read") || !g.repositoryToken(r, name, "contents", "read") {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.repositories[name]; !exists {
		w.WriteHeader(http.StatusNotFound)
		return true
	}
	g.proof.BranchReads++
	if g.branchFaults[name] > 0 {
		g.branchFaults[name]--
		g.proof.BranchFailures++
		w.WriteHeader(http.StatusServiceUnavailable)
		return true
	}
	head, exists := g.branchHeads[name]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return true
	}
	lifecycleJSON(w, map[string]any{"ref": "refs/heads/main", "object": map[string]any{"type": "commit", "sha": head.SHA}})
	return true
}

func (g *lifecycleGitHub) writeExternalWork(w http.ResponseWriter, r *http.Request) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/github/repositories/"), "/")
	if r.Method != http.MethodPost || len(parts) != 2 || parts[1] != "commits" || !lifecycleRepositoryName.MatchString(parts[0]) || r.Header.Get("Origin") != "https://github.com" {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	cookie, err := r.Cookie("lifecycle_github_browser")
	if err != nil {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	var body struct {
		ParentSHA string `json:"parent_sha"`
		Content   string `json:"content"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || body.Content == "" || len(body.Content) > 4096 {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	user, authenticated := g.webSessions[cookie.Value]
	name := parts[0]
	repo, exists := g.repositories[name]
	if !authenticated || !time.Now().Before(user.Expires) || !exists || g.collaborators[name][int64(5000+user.Actor)] != "write" {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	previous := g.branchHeads[name]
	if previous.SHA != body.ParentSHA {
		w.WriteHeader(http.StatusConflict)
		return true
	}
	content := sha256.Sum256([]byte(body.Content))
	receipt := lifecycleExternalWorkReceipt{ID: uuid.New(), RepositoryID: repo.ID, Name: name, ActorID: int64(5000 + user.Actor), ParentSHA: body.ParentSHA, ContentSHA256: hex.EncodeToString(content[:]), CreatedAt: time.Now().UTC()}
	raw, _ := json.Marshal(receipt)
	head := sha256.Sum256(raw)
	receipt.SHA = hex.EncodeToString(head[:20])
	if g.branchHeads == nil {
		g.branchHeads = map[string]lifecycleExternalWorkReceipt{}
	}
	g.branchHeads[name] = receipt
	g.proof.WorkTransactions++
	w.WriteHeader(http.StatusCreated)
	lifecycleJSON(w, receipt)
	return true
}
