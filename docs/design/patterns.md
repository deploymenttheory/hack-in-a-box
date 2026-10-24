# Fleet collection and decision patterns

DS-03 (#232) builds on the [visual contract](visual-contract.md) and
[primitive API](primitives.md). Domain screen migrations consume these patterns
in their own tickets. This draft introduces shared composition and synthetic
verification, without changing backend contracts or portal domain screens.

Import production patterns from `web/src/components/patterns/index.ts`:

- `CollectionHeading`: named scope, one primary action, quiet secondary actions.
- `CollectionToolbar`: explicit complete/cursor source and optional owner search,
  filter and column controls. Cursor sources permit server search only.
- `CollectionRecords`: native table at wide widths, labelled record list at narrow
  widths, stable native links, controlled sort/columns/eligible page selection.
- `CollectionPagination`: opaque next/previous callbacks and loaded counts; totals
  are displayed only when the owning API supplies them.
- `CollectionFrame` / `CollectionStateMessage`: distinct loading, empty, no-match,
  failed, paused, offline, stale and denied states. Definite denial hides private
  heading, toolbar and records. Temporary failure retains mounted values and
  pauses activation. Key the frame by the owner's identity/session/event scope.
- `DetailWorkspace` / `Inspector`: a wide detail pane becomes the selected detail
  canvas on narrow screens. Supply the shared owner state to keep one visible
  failure banner above a selected detail; definite denial removes both panes.
  URLs, browser history and back context remain owned
  by the router adapter. `Breadcrumbs` / `ContextLink` retain native anchors.
- `ReviewFlow` / `CommandState`: prepare, review/correct/cancel, deliberate confirm,
  exact receipt and relevant next action. Unknown outcomes expose a read callback
  and retained intent ID, with no generic replay callback. Owner wording must
  distinguish request recording from provider acceptance or domain completion.
- `ActionQueue`: supplied tasks and states, with quiet labelled actions. Selection
  can queue individual reviews; it never synthesises an API bulk operation.
- `CommandDestinations`: searches only an already permitted navigation registry;
  no private record/cache index or global key binding is consulted.

Patterns own presentation, keyboard semantics, density, responsive composition and
focus. The owning adapter retains generated API calls, CSRF, revision checks,
current authorization, payload/receipt matching and private data lifecycle. Use
`PrivateRequestScope` or the existing owning-domain fence for asynchronous work.
Frame keys do not authorize a request or clear state stored above that frame.
When current roster capability is lost, redact privileged unknown-command payloads
from event-lifetime history while retaining minimal intent metadata, original revision
and the unresolved write barrier. An initial participant projection must not erase
ordinary unknown join/create intent merely because it has no management capability.

## Capability examples

Events support cursor paging without general server search or totals. Released
content supports authorized `q` search and a UUID cursor. Inbox supports event
filtering, cursor paging and authoritative unread count. Audit supports numeric
cursor paging and a real bounded export. Repository mappings are an authorized
array, while repository access review has separate exact receipt/current state.
Expose controls for the owning API's capabilities; never search only one server
page while claiming a global result, or fabricate a total/download/bulk command.

Optional column preferences contain column IDs only. Selection is explicit IDs
on the visible eligible page, pruned on changed source and reset on query, cursor,
account/session/event replacement. Owners preserve safe navigation context and
use their established unsent/uncertain intent guards.

## Shared guide examples and verification

The guide can import `PatternExamples` directly from `components/patterns/examples`
with `withinTheme` when the guide supplies its own authoritative Theme/portal scope,
and pure fixture functions/types from `components/patterns/scenario`. These modules
use synthetic people/records and a backend-free owner transport. They are separate
from the production pattern barrel and contain no production API client calls.
Current-version reads cannot impersonate exact receipts; stale versions need an
explicit current-source read and a new review. Late work is fenced on scope loss.

`web/e2e/pattern-harness` imports these actual production patterns into an isolated
Vite build. Neither portal router/build includes the harness, its controls or its
synthetic legacy-card baseline. `npm run test:e2e:patterns` verifies admin and
participant scenarios at desktop/mobile sizes on its dedicated local preview.
The static guide can use the same examples without a guide-to-test dependency.

The [fresh verification record](../verification/fleet-patterns/README.md) retains
local results, corrected failures and representative synthetic visual artifacts.
Exact-head repository CI and independent review remain required. Domain screen
migrations and the final parity audit remain separate backlog work.

Narrow records preserve owner-supported sorting with labelled native buttons for visible sortable columns, active direction and disabled state. Rows are never reordered by the pattern; the owner supplies their resulting order. Repeated deliberate synthetic searches and clears issue a fresh scoped read even if the query and cursor are unchanged.


## Local records without a route

Omit CollectionRecords.destination for local editor selections that have no
addressable route. The record name remains text; supply a labelled native button
through actions. Routed records retain the existing real anchor/href semantics.
Do not invent fragment/bookmark destinations for a domain screen that cannot
restore them. Owners retain dirty/uncertain navigation guards and return focus
when their inspector closes. Set data-record-focus to the row key on the primary
local action when it must preserve focus across a responsive breakpoint. The
interactive guide includes the actual production composition and keyboard example.


## Team and mentor planning

Team/mentor collections use complete authorised projections. Search and counts apply
only to that projection; published coverage never implies access to other teams or
private assessments. Use a concise named detail beside the roster; allocation,
placement, profile editing and publication reviews need the full canvas. Keep the
reviewed revision, exact affected teams/slots/windows and recorded reason visible.

`coverage-command.tsx` is the bounded adapter for the native team/coverage page-response
APIs. Their responses contain no durable command receipt. Successful explicit responses
are labelled server responses; an interrupted/timeout/5xx write retains its local
diagnostic intent and permits current-state reads only. A later revision cannot settle
that original intent. Read-only back navigation does not release the write barrier.
`coverage-detail.tsx` uses a real local Back button and focuses its heading; the owner
restores the initiating record/action focus when it closes. The interactive guide's
roster example imports these production components with synthetic in-memory state.


Roster adapters bind privileged editor and reviewed snapshots to the owning API's
current `can_manage` projection, independently of the parent event query. A lost
capability replaces that local lifetime immediately; child resolution tickets and
parent projection generations fence held continuations and late private responses.

Capture the real initiating control through `useCoverageFocus`: stable record/action
keys restore the rendered counterpart after viewport changes; a unique native action
name identifies a heading action. The root is a named focus fallback when the original
action no longer exists. Keep nested editors mounted hidden/inert during correction
review, or lift their drafts. Report actual nested inputs and review separately from
pending/unknown commands; `useCoverageDraftGuard` confirms deliberate local/route
discard without unlocking an uncertain write. A rejected discard retains exact input.

For a definite collection-wide denial, the collection owner must clear its private selection and affected cached projections and invalidate pending read lifetimes above any child that it disposes. Restoration requires fresh native authority; never reopen a cached child merely because its parent list is readable again. Keep event/session unknown-command metadata and barriers separate from private payload: `usePlanningUnknownRedaction` must match an exact owned actor/draft/event/resource namespace, redact private titles/commands, retain original unknown revisions/comparison requirements and leave unrelated histories untouched. Redaction is neither a receipt nor permission to replay.
