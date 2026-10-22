# Clarification workspace verification

The #162 workspace uses native private question, immutable answer and actual
publication APIs. The resource library and organiser content workspace link to
`/events/<event-id>/clarifications`; authorised private questions have their own
`/events/<event-id>/questions/<question-id>` route. Existing canonical answer and
inbox destinations remain valid. Future Today links can use these same routes.

Common search contains published answers only. Author, publication time,
correction history and exact challenge/rubric versions come from the native
reader. Confirmed attendees can read the existing public rubric projection;
staff-only sessions do not fetch that confirmed-only endpoint or private judge
configuration. The new staff preview is a server-derived current confirmed
recipient count under the same event lock and source policy as publication.

Questions, attachments and unpublished answers remain in private routes and
ephemeral person/session/event/resource caches. Unsent editor text and upload
bytes stay in page memory only. Navigation warns before clearing unsent or
uncertain work. Current native denial, account switch and event switch remove
private forms and cached content; private sections are excluded from print.

An interrupted question submission offers an explicit retry of the identical
command ID and payload. Interrupted answer writes never replay automatically:
the editor reconciles exact document/source/version/revision, distinguishes a
confirmed mutation from a failed follow-up read, and requires explicit comparison
before adopting a competing server version. Save, approve and publish are
separate decisions. Material-change labels require the actual owning release,
impact review and notice; they cannot modify criteria through an answer.

## Evidence

- Real PostgreSQL/race workspace and strict HTTP tests verify current released
  source, own-team authority, staff-only actual audience, nullable empty draft
  versus denied review, withdrawal, role revocation, archive and no-store/privacy.
- Thirteen browser scenarios run on desktop and mobile in both independently
  built web surfaces. They cover asker, other-team reader and staff, embargo,
  correction/history, keyboard controls, UTF-8 validation, actual attachment
  download, live attachment gating, print privacy, explicit same-command retry,
  unknown save/source changes, competing versions, archive, current review-role
  loss, account/event cleanup and confirmed-write/read-failure reconciliation.
- Final composition includes existing help, mentor, recovery and canonical
  clarification readers after rebasing onto merged help workspace #206.
- Both builds, web lint, CI-equivalent Go lint and regenerated Go/TypeScript
  contract parity pass. No database migration is introduced.

Exact-head CI and source review remain merge gates. This work supplies the native
destinations consumed by the future Today dashboard; it does not implement #88.
