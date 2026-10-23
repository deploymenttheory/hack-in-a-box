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
