# Mentor administration

Issue #67 adds a Mentors section to the independent admin portal and participant
app. Current organisers get the team/mentor matrix in the admin portal. The
participant build imports only the published roster, with no management controls
or roster-editing queries, even for an organiser identity. Permitted competitors
and mentors get only the published roster returned for them by #66's
current-authority API. That API remains the
authority for every read and command. The roster refetches on window focus and
section remount, and permission failures replace the cached display.

The matrix separates primary and second-review slots and exposes declared team
skills, availability, coverage gaps and each mentor's published versus draft
workload and capacity. Organisers can edit event capacity, personal overrides,
declared skills, available intervals and known relationships. References identify
teams/people when names repeat. Narrow screens use a keyboard-focusable horizontal
scroll region with readable table columns.

Availability and assignment editors resolve event-local date/time inputs through
the existing server time resolver, including explicit repeated-time choices.
Assignment preview shows every mentor's before/after workload, the affected team's
slot and selected interval, partial primary coverage and uncovered slots. Known
conflicts, duplicate primary/second people, capacity, out-of-live intervals and
unavailable periods prevent saving. The backend revalidates all constraints.

Every editor captures the event revision at opening. A changed roster disables
the edit and requires cancelling/reviewing current state. Generating/dismissing a
proposal writes nothing; applying one saves a draft. Publishing needs a separate
revision-bound preview and decision reason. Soft coverage gaps require explicit
acknowledgement with that reason; hard constraints cannot be waived. Cancelling
publication writes nothing. Changed/stale publication invalidates the preview.

Published and draft rosters remain separate. Declaring a known conflict may revoke
current access immediately, so that action explains the effect. Replacement and
publication operate on mentors only; competitor freeze remains in force, and the
UI never offers competitor moves as a workaround for a depleted frozen team.

Desktop/mobile browser scenarios cover keyboard assignment, workload and coverage
previews, old published membership until explicit publication, gap approval,
capacity and declared conflicts, profile availability, stale publication,
read-only permitted views and revoked access, configuration edits, and proposal/
publication cancellation. Implementation PR evidence records the complete web,
backend and image checks. Real PostgreSQL authorization/publication tests are
owned by #66 and the combined race scenarios by #68.
