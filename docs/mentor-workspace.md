# Mobile mentor workspace (#100)

The directly addressable `/events/:event/mentor` overview and team route use the
existing authenticated shell independently of Today. Coverage comes from the
current native mentor projection: active role/profile, published assignment and
conflicts. Each team's current observation API remains authoritative for reading,
formal review eligibility and writes. No generic participant team browser or
organiser-only directory supplies the overview.

Private category/evidence/outcome/feedback reference and operational status are
labelled mentor/organiser-only. Neither publishes participant feedback or changes
a score. Entry explains scheduled live start and actual common publication.
Native `can_record` and `can_change_status` control their distinct forms. Original
authors can edit their observations; timestamps and immutable history remain
visible to current authorised readers. Notes and histories traverse native
cursors. The timeline orders the loaded records by observation time; more
records are explicitly loaded rather than silently truncated.

## Recovery and deliberate writes

Every unsent text change uses the #89 allowlist and absolute 30-minute local-copy
policy. Feedback references are deliberately excluded from local persistence; a
reference-only edit still creates the text-only copy needed for guarded saving.
Local, unconfirmed and server-confirmed outcomes are distinct. No local copy is
labelled saved on the server. A native write's uncertain response does not trigger
automatic retries: an explicit exact-UUID lookup compares current server evidence
and reference before confirming or offering a new deliberate choice.

Recovery checks current authority/live eligibility and the exact saved note's
native history, separately from its current-page position. A known existing
note's 404 never becomes version zero. An explicitly new client UUID may not yet
exist. Before each write, a fresh GET/revision plus note version is required; a
changed pair needs a deliberate comparison. Another concurrent change requires
another choice. Native If-Match and original-author enforcement remain final.

Current coverage, team readiness and exact-note queries refresh every 15 seconds
while active and on focus. A current native team denial or closed formal-write
window removes only that session/person/event/team's recovery copies. The editor
unmounts or clears, including restored in-memory text. Transient network errors
hide current private views but retain the bounded local copy; they are not
revocation. Session changes and cross-tab logout retain the stronger #89 cleanup.

Operational check-in edits retain their original event/status revision. An unseen
newer check-in is displayed for review and requires an explicit new base; a
background GET cannot silently rebase the old intent. Operational reasons/actions
are not persisted in the observation-text recovery store.

## Verification boundary

Browser fixtures exercise actual mentor routes/forms with deterministic native
API responses and transport faults. Existing real PostgreSQL/race tests verify
current assignment/conflict/role policies, same-author concurrent edits, live and
common-release gates, pagination, isolation and the authenticated HTTP contract.
The workspace introduces no schema, backend mutation or scoring endpoint. #102
owns scorecard controls; #103 owns any future shared/released action projection.

#89 aggregate acceptance is reconciled only after the actual #100 interrupted
save, refresh/team-switch, expiry/session, exact-version conflict and recovered
pre-live journeys have been independently reviewed and merged.

Local verification at the implementation checkpoint: 28 actual-workspace
desktop/mobile browser cases pass in 8.4s; 16 recovery unit cases pass. Real
PostgreSQL/race checks pass observations8.962s, current mentor roster2.877s and
authenticated observation HTTP3.604s. Both builds and frontend lint pass.
