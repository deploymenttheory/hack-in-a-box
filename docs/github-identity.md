# Verified GitHub identity

Organisation OIDC signs the person into Hack-in-a-Box. GitHub OAuth separately
proves control of a GitHub account. Account settings shows the verified immutable
GitHub ID and current login, never a token. A typed login is not proof.

## Deployment

Create a GitHub OAuth application with the exact callback
`HACKBOX_PUBLIC_ORIGIN/auth/github/callback`. Configure these together:

- `HACKBOX_GITHUB_CLIENT_ID`
- `HACKBOX_GITHUB_CLIENT_SECRET`
- `HACKBOX_GITHUB_TOKEN_KEY`: a separate random 32-byte AES key, base64 encoded.
  Generate it with `openssl rand -base64 32`, then store it with deployment secrets
  and backups. Never commit the output. Keep the same key across API replicas.

Production requires HTTPS through the existing public-origin validation. Missing
GitHub configuration disables linking and explains that state on the account
screen. Removing provider configuration does not prevent unlinking an existing
account. Losing/changing the encryption key requires users to verify again to
replace stored credentials; automated rotation and repository delivery belong to
later integration work. Do not restore encrypted records without the matching key.

The initial integration uses GitHub.com and requests no repository or private-user
scope. `/user` still identifies the authenticated token owner with its public
response. Repository automation will use its separately configured GitHub App;
this OAuth identity must not become a substitute for installation authorization.

## Verification and invitation impact

A CSRF-protected POST starts authorization after the person acknowledges the
invitation impact. State is hashed, expires after five minutes and is consumed
once, bound to the same corporate person and browser session. S256 PKCE protects
the code exchange. The callback always returns to `/account`; callback parameters
cannot select a different destination. Corporate session revision, continued
session existence and link revision are checked again before persistence.

Tokens (including any returned refresh token and expiry) are encrypted with
AES-256-GCM and authenticated against the corporate person's ID. Token refresh is
not needed to display the verified identity; future provider operations must
verify token validity and current permissions before use. Provider errors and
tokens are excluded from HTTP payloads, application logs and audit metadata.

A renamed login updates the display name while retaining the immutable ID. A
changed ID or unlink flags pending `github_invitation_bindings` as
`requires_review`, preserving their original ID. It does not transfer or revoke
existing repository access. Delivered bindings stay as delivery history. The
future repository-invitation handler (#93) must create a binding under the person
lock, require its current ID/revision and `pending` state immediately before sending,
and reconcile provider ambiguity. This batch does not send invitations.

## Primary references checked during implementation

- [GitHub OAuth authorization](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps): state, redirect URI, S256 PKCE and token exchange.
- [Authenticated user endpoint](https://docs.github.com/en/rest/users/users#get-the-authenticated-user): no-scope public identity response and immutable account ID.
- [google/go-github](https://github.com/google/go-github): pinned SDK v89.0.0;
  client construction verified against the installed source.

Real PostgreSQL tests use a local deterministic OAuth/API provider. They cover
forged/cross-person/cross-session callbacks, replay, expiry, logout/revocation,
renames, duplicate ownership, encrypted storage, unlink/relink and invitation
review. HTTP and browser tests verify CSRF, safe redirects, absent tokens,
acknowledgment, disabled configuration and mobile layout. A real GitHub application
must be configured in the deployment before a live provider smoke test.
