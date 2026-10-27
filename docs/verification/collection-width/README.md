# Readable collection panes (#337)

The connected native lifecycle R50 exposed a many-column form table squeezed
into a narrow pane inside a 1440px desktop viewport. The independently inspected
[before image](before-native.png) uses the actual normal-OIDC form-authoring
journey and synthetic data. It is not a generated mockup.

The native-browser regression reproduces a 400px collection inside that wide
viewport. Before the fix it fails because the table never becomes labelled
records. After the fix it passes with record, page-selection and sort focus
preserved, selection retained, no horizontal page overflow, and external toolbar
focus left alone. All56 pattern browser checks pass across both portals and
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
