# Team credential storage and recovery

Issue #73 stores organiser-supplied credentials. It does not provision provider resources. Organisers can create, replace, assign, revoke and rotate credentials using `/api/v1/events/{event_id}/credentials`; mutations require current organiser authority, CSRF and the observed event revision in `If-Match`. Assignment replaces the complete team audience. Replacement preserves assignments and revoked state. Revoked resources remain revoked; create a new resource to distribute a new credential again.

Use safe resource labels. Never paste a credential into a label, URL, support message or decision text. Metadata endpoints include no value, ciphertext or key identifier. Members see only their assigned active resources and no other team's assignment list. An organiser's role permits management but does not itself permit disclosure. Explicit `POST .../{credential_id}/reveal` requires a current confirmed competitor in an assigned published team with an active competition entry. Withdrawal, published reassignment, team disbandment, inactive identity, resource revocation and assignment removal immediately remove reveal access. Draft membership grants nothing, including after team freeze.

Reveal responses and errors use `Cache-Control: no-store`. Accepted reveal attempts record actor, resource, database timestamp and bounded result (`success`, `denied`, `not_found`, `conflict`, `unavailable`). Authentication/CSRF/contract failures are rejected before the reveal operation. No audit value contains the credential. A disclosure only returns after its audit transaction commits. An audit-storage outage prevents disclosure. A missing key, wrong key, corrupted ciphertext or context mismatch returns a generic unavailable response and commits an unavailable audit result. Network retries are separate explicit reveal attempts and each produces its own audit record.

## Deployment configuration

Set `HACKBOX_CREDENTIAL_KEYRING` separately on both public and admin API deployments. Its JSON shape is `{"active":"key-2026-10","keys":{"key-2026-10":"<base64 of 32 random bytes>"}}`. Key identifiers are 1–64 ASCII letters, digits, underscores or hyphens. The `active` identifier must exist. Generate key material using an approved cryptographic random generator and distribute it through the deployment secret manager. Do not commit real values to source control or put them in command history. Compose passes this setting through; production deployments should inject it from secret storage. Malformed configuration prevents startup without echoing supplied values. An absent setting permits ordinary app startup and safe metadata access but credential create, replace, rotation and reveal fail closed.

The AES-256-GCM envelope uses a fresh random nonce for each write. Authenticated context binds ciphertext to its event, resource ID and value version. PostgreSQL contains only the ciphertext and key identifier. The reusable internal `secretbox` package owns encryption only: separately authorised integration secrets may use it with their own context and policy; they must never use team credential endpoints.

## Rotation

1. Back up PostgreSQL and the existing keyring separately in protected storage. Preserve all old keys needed by live data and retained database backups.
2. Add a fresh key with a fresh identifier to both deployments, keeping all existing keys. Select it as `active`. Existing ciphertext remains readable with old keys; new writes use the new key.
3. As a current organiser, read credential metadata and issue an explicit `rotate` command for each resource ID against the latest returned event revision. The operation decrypts with its recorded old key and re-encrypts under the active key with a new value version. It atomically updates ciphertext, appends `credential.rotated` audit evidence and advances the event revision. It never returns plaintext. A concurrent update returns conflict; reload and review before retrying. Revoked resources can also be rotated without restoring access.
4. Confirm every event's `team_credentials.key_id` uses a retained key before retiring an old key. Keep old keys for as long as retained database backups require them. Key retirement is a deployment operation; do not silently remove keys as a side effect of rotation.

If an old key is accidentally removed, restore the exact original key material under the original identifier. Rotation failures leave ciphertext, version and event revision unchanged. Never overwrite an identifier with unrelated material. If ciphertext is corrupted, recover the matching ciphertext and version from a protected backup; do not swap records across resources or versions. If recovery is impossible, an authorised organiser can replace the supplied value using an intact active key; replacement does not require decrypting the old value and preserves existing audiences and revoked state. Review the provider credential separately before redistributing it.

App revocation removes only app disclosure access. Someone who previously revealed a value may retain it. Organisers must revoke or rotate the actual provider credential independently when needed; the app never claims that app revocation performs that action.

## Verification

Real PostgreSQL tests cover current confirmed published membership, withdrawal, cross-team draft isolation, retired/disbanded teams, inactive identities, audience/resource revocation, stale revisions, ciphertext tampering, missing keys, rotation rollback/recovery, replacement and audit-storage failure. HTTP tests validate the generated contract, no-store responses, explicit-only disclosure and absence of values in metadata, audit and request logs. Encryption tests cover random nonce uniqueness, wrong-key authentication and event/resource/version context substitution.


## Restoring a database without deployment keys

A PostgreSQL backup is insufficient to recover credential values. Restore it into
an empty database, run the matching schema check, and configure authentication
before granting users access. Safe resource metadata remains available without
`HACKBOX_CREDENTIAL_KEYRING`; deliberate disclosure returns unavailable and records
an unavailable audit result. Do not replace ciphertext, clear assignments or
remove availability/expiry controls to make that error disappear.

Recover the exact protected key material under every key identifier required by
the restored rows. An unrelated key under the correct identifier cannot decrypt
the backup. Selecting a fresh active key also does not recover older ciphertext.
Retain the original keys alongside the new active key, verify deliberate disclosure
through current team authority, then rotate resources with current organiser
permission and revisions. Only retire older keys once live ciphertext and all
retained backups no longer need them. Database restoration does not revoke actual
provider credentials or invalidate copies that users obtained previously.

`TestCredentialLogicalRestoreRequiresExternalRetainedKeys` runs `pg_dump` and
restores its SQL with `psql` into a new empty database on isolated PostgreSQL. It
checks that the dump includes ciphertext but contains neither supplied values nor
deployment keys; verifies preserved envelopes, audience and time metadata; and
exercises missing keys, wrong material, missing old keys, retained-key recovery and
rotation followed by old-key retirement. Source writes after backup remain
independent of the restored database. Key failures must leave values empty and
commit redacted audit evidence.

The HTTP confidentiality check keeps an existing authenticated session through
assignment removal, published reassignment, absent/draft membership and
withdrawal. Direct requests must follow current authority. Protected CSV/HTML
exports, metadata, logs and audit records are checked with synthetic values;
responses remain `no-store`. A populated approved room snapshot excludes resource
metadata and values, and its capability cannot authenticate to credential list or
reveal routes. Participant browser storage and temporary visibility
are covered by the controlled reveal interface tests.
