# Normal production-image verification at794214f5

All four desktop/mobile × light/dark cases passed using the normal public/admin
commands, fresh PostgreSQL schema55 and an external synthetic OIDC provider.
No API interception, seeded session or application test clock is used. Images
were built from the clean tracked source archive; exact labels/image IDs and all
22 original PNG hashes are in native-normal-ui.json. Images are copied unchanged.

Every case creates a native draft event through UI review/confirmation, reads it
in both independent portals as the same organiser, uploads an actual photograph
(ACK1), renders its protected49,909-byte PNG thumbnail, explicitly approves
(version1→2), releases(2→3) and withdraws(3→4). Each version comes from the exact
original PUT acknowledgement. Later intents use the explicit current-metadata
confirmation; current reads never substitute for write acknowledgement. A fresh
thumbnail read after withdrawal returns403, including for this organiser.

All cases verify separate logout, zero guide API requests/cookies and no page
errors. Captures check horizontal overflow and serious/critical automated axe
findings; these checks do not replace full accessibility signoff. The final case
stops the public process and still reads admin and guide. Both app readers are
the same organiser; this is not a confirmed-competitor or complete lifecycle proof.
The image has no identifiable people or challenge content; tagged-person consent
revocation remains covered by the native media HTTP suite rather than this journey.

cleanup.json records empty container/volume/network inventories for this unique
project. The existing user installation and unrelated databases were preserved.
