# Readable collection panes (#337)

The connected native lifecycle R50 exposed a many-column form table squeezed
into a narrow pane inside a 1440px desktop viewport. The independently inspected
[before image](before-native.png) uses the actual normal-OIDC form-authoring
journey and synthetic data. It is not a generated mockup.

The native-browser regression reproduces a 400px collection inside that wide
viewport. Before the fix it fails because the table never becomes labelled
records. After the fix it passes with record, page-selection and sort focus
preserved, selection retained, no horizontal page overflow, and external toolbar
focus left alone. All60 pattern browser checks pass across both portals and
mobile/desktop contexts; all27 shared-pattern unit checks pass. Both portals,
the independent guide and lint build/check successfully.

The [after image](after-route-fixture.png) is the actual production form-editor
route with explicitly mocked API fixtures, using a different synthetic form. Its
browser test exercises conditional-question validation, preview, draft save and
publication acknowledgement. This is route/component evidence; it is not a
normal-OIDC/native API result or a complete #120 acceptance claim. A connected
native rerun with the component change will be recorded separately.

Reproduce the focused layout regression with:

```sh
npm --prefix web run test:e2e:patterns -- --project=admin-desktop -g 'narrow collection panes'
```

Reproduce the form-route evidence after building both portals with:

```sh
npm --prefix web run build
npm --prefix web run test:e2e -- --project=desktop e2e/registration-forms.spec.ts -g 'form editing validates'
```

The interactive guide's Collection pane width control imports the production
component. Usage, source snippet and keyboard guidance are updated together.
No application API, current authority, private-data or save/publication contract
changes. Full #120 and #123 release gates remain open.


Independent review also found a valid-key focus collision (`page` and
`sort:name`). A second real-browser regression reproduces the wrong focus
counterpart before correction, then passes after internal page/selection/sort
identities use a separate attribute and unambiguous tuple encoding. Existing
owner record keys remain untouched. All60 shared-pattern browser checks pass
with that correction; the original narrow-pane red result remains distinct.

A native R51 image then exposed an incomplete visual result: the list was
visible, but a long action still squeezed the long record name. The targeted
form-route regression measured only39.4375px for the name (minimum160px in the
fixed desktop fixture). Wrapping the record header and giving the title a
readable flex basis fixes that case; the same long-name route/save/publication
test now passes. The independently inspected route image above is refreshed
with this long-name fixture. Native R51 was intentionally stopped after that
verified visual defect; it is not counted as a passing readability result.

The style-guide CI failure was reproduced separately: a newly visible record
heading and its inspector heading shared a name, making the old focus assertion
ambiguous. The test now names the inspector region and still requires its
heading to receive focus. The complete independent guide browser suite passes
all86 desktop/mobile checks after that correction. Native verification with the
long-name gate and final wrapping change remains required before ready/merge.
