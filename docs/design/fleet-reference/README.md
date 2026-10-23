# Accepted Fleet UI reference

The user accepted `fleet-preview.html` on 23 October 2026, replacing the earlier
presentation-preserving direction in PR224. [Backlog #227](https://github.com/deploymenttheory/hack-in-a-box/issues/227)
owns the greenfield refactor; #230 establishes its visual contract. These are
synthetic review prototypes, not production API clients or finished product pages.

Open `fleet-preview.html` to inspect Workspace, Review desk and Action queue across
admin/participant roles and event phases. They share one visual language; production
uses each pattern where it fits the task, rather than exposing a layout switcher.
The HTML fragment, editable source, compiled tokens, original verification record
and twelve desktop/mobile screenshots are retained byte-for-byte. The original
preview wrapper expects its Lucide/helper resources; screenshots and the editable
source remain inspectable without them. The future production guide (#233) will
use the application's own components and local assets without a backend.

`manifest.json` records SHA-256 digests of every archived artifact, including the
accepted HTML. `quality-coverage.csv` maps all 100 qualities to foundation,
guideline and migration owners. The map is an applicability contract, not a claim
that all product pages already implement these patterns. Tables suit comparable
collections, prose suits reading, images suit galleries and a projector requires
distance-readable content. Existing closed features retain their backend/domain
meaning while their UI is rebuilt in the new issues.

The [100-quality study](fleet-study/100-ui-qualities.md) pins Fleet's source revision
and links primary evidence. Observed Fleet facts and our design adaptations are
separate columns. We do not copy Fleet branding, component code or screenshots
into the product. The PNGs here depict our own synthetic proposals. Inter is a
separately sourced self-hosted font with its OFL license and provenance under
`web/src/design-system/fonts` and `web/public/assets/licenses`; it was not taken from Fleet assets.

The archived prototype verifier result describes its original local checks, not
the current product's browser/accessibility or release certification. New production
verification is recorded separately in the implementation PRs and #122/#120.
