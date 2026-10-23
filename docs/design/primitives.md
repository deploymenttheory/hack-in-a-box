# Fleet accessible primitives

Use these production exports with the [visual contract](visual-contract.md).
The interactive guide (#233) imports these same components; examples must not
create an alternative UI kit. CSS lives in `design-system/primitives.css` and
consumes the existing semantic tokens. No primitive changes authority, submits a
request, persists input, retries an operation or interprets a provider receipt.

## Actions and icons

`Icon` accepts a curated `IconName`; `iconNames` is the readonly catalogue derived
from that registry. Named Lucide imports keep glyphs local and tree-shakeable.
Icons use the shared 16px/1.75/current-colour contract and are decorative: the
surrounding control or visible status supplies its accessible meaning.

`Button` retains native button/ref/form semantics and existing CVA variants.
Use an explicit `type="submit"` for a form action and verb-first text with a
meaningful `icon` for primary actions. `pending` disables activation, sets
`aria-busy` and replaces the supplied icon in the same slot without changing the
label or dimensions. Supplying the `pending` boolean without an icon reserves
the same icon-sized indicator slot in the idle state; busy actions show its
spinner while keeping their visible label. Default buttons without this contract
retain their compact spacing. A caller still guards its exact intent synchronously;
disabling a button does not reconcile an unknown write. `IconButton` requires a
`label`. `ActionLink` renders an anchor with `href`; use router render composition
where the router owns navigation. Do not turn navigation into an onClick write.

```tsx
<Button type="submit" icon="save" pending={pending}>Save draft</Button>
<IconButton icon="close" label="Close review" onClick={close} />
<ActionLink href="/account" icon="user">View account</ActionLink>
```

## Native fields and validation

`Field` takes `id?`, `label`, `description?` and the owning validator's `error?`.
It gives native `Input`, `Textarea` and `Select` a stable ID and appends help/error
IDs to any caller-provided `aria-describedby`, without duplicate or absent IDs.
Put an explicit ID on `Field`, not a different ID on its control. Refs and native
attributes pass through, including React Hook Form registration. `Checkbox`
remains binary; a domain's unknown/yes/no value remains a three-state select.
Groups retain a native fieldset/legend and explicit group descriptions.

```tsx
<Field id="team-name" label="Team name" description="Use the published name"
  error={errors.name?.message}>
  <Input {...register('name')} aria-describedby="additional-guidance" />
</Field>
```

Native forms, RHF/Zod and the owning domain remain the validation boundary. In
particular, async Base UI visual validation must not authorise submission. On an
invalid submit focus the first usable invalid control after errors render;
`focusFirstInvalid` skips disabled fieldset descendants, hidden/inert controls
and nonfocusable invalid wrappers. Keep field errors and transport errors
separate. Field error focus/description announces the current message without
adding an assertive live region to every field or every keystroke. Error styling
uses a constant border width with an inset indicator to preserve geometry.

## Scoped overlays and decisions

`Menu`, `Popover`, `Tabs` and `Dialog` wrap the installed Base UI primitives.
Their content uses `Theme`'s local portal host, never a body fallback while the
host is initially absent. This includes initially open content. Menu and popover
items use the same semantic text/focus/target tokens; disabled menu items remain
unavailable. In forced-colour mode, highlighted and keyboard-focused menu items
use a 2px inset system `Highlight` outline; the indicator does not depend on
background colour or box-shadow. Tabs activate manually with Enter/Space after arrow-key focus.
Route navigation remains links rather than fake tabs.

`DialogContent` requires a title, accepts a description/footer and forwards
`initialFocus`/`finalFocus`. Base UI owns modal focus trapping, Escape and focus
return. `ConfirmationDialog` is controlled and accepts `finalFocus` for callers
without a Base UI trigger; cancellation receives initial focus. Pending
confirmation blocks dismissal and duplicate activation. The owner decides when
the request has settled and supplies a useful focus target when closing; the
primitive does not close itself on a promise or retry an uncertain operation.

`Tooltip` supplies an additive accessible description for an already named
control and appears on keyboard focus as well as hover. Essential guidance must
remain visible outside it. Touch users must not need a hover-only explanation.

## Informational feedback

`StatusBadge` always includes text and a decorative glyph, and never creates an
implicit live region. Tone is not proof of permission, acceptance or scoring.
`Progress` requires a label; missing/invalid measurements stay indeterminate
rather than becoming zero. `LiveMessage` is an explicit atomic status (or urgent
alert), used only for meaningful state changes.

`ToastProvider` is local and requires an account/event `scopeKey`. Replacing the
scope resets its manager and notifications. `useToast` accepts safe text, not
raw exceptions; urgent notifications persist until dismissed. Critical guidance
and exact receipts also remain inline. No global manager, storage persistence,
automatic write replay or private-value echo is introduced.

The initial production consumers are the account menu, registration questions,
event setup error relationships, shared loading/failure output and Teams TEST
confirmation. Remaining domain composition belongs to its migration owner; a
control migration does not reopen backend features or replace recovery rules.
