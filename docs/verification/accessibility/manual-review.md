# Manual keyboard, assistive-technology and device procedure

No execution is recorded by this document. Use synthetic events/accounts and
record the exact built source, browser release, operating system, device,
viewport/zoom, assistive technology/version, date, steps, actual result, artifact
and defect/retest links for every run. Do not record real participant information,
keys, cookies, headers or private production text.

## Start and environment

1. Use the final matching schema/backend and separate production portal builds.
   Verify the independent guide remains reachable if the regular portal fails.
   Record actual source/build identity and current theme/palette.
2. Record the installed **released browser** and version, not only a driver or
   project name. Check current stable eligibility against its official release
   source at execution. Keep upstream engines and simulators separately labelled.
3. Use an identified actual mobile Safari and mobile Chrome environment for the
   mobile rows. Record OS/device/browser and virtual keyboard behaviour. Desktop
   width/touch emulation supplements these records rather than replacing them.
4. Enable and operate the chosen screen reader deliberately in the review
   environment. Record what it announces. DOM/ARIA snapshots are not spoken output.
   A browser/screen-reader combination is an execution record, not a new product
   requirement beyond the ticket's existing supported targets.

## Run every journey

For each row in `journey-review.json`, start at its actual entry route and
complete the primary action, review, correction, result and next action. Include
current denial, stale data, network/field errors and uncertain effects where the
owner supports them. Final judging/practice/showcase rows wait for real completed
owners; do not substitute a prototype or mark them passed because unavailable.

### Keyboard

- Put the pointer aside. From browser entry, use Tab/Shift+Tab, Enter/Space,
  arrow keys and Escape as appropriate. Record reachability, order, visible focus,
  scroll and focus obscuration. Confirm no keyboard trap.
- Reach the primary action through real navigation, correct a review, confirm an
  allowed synthetic action, then follow the result to its next action. Check
  source focus after closure and after native asynchronous refresh.
- Exercise menus, tabs, responsive section selectors, tables, dialogs, upload,
  print/export and search/no-match recovery. Check that drag interactions have
  usable alternatives and that current status has text/semantics beyond colour.
- Verify stale/unknown commands remain explicit and are not replayed by navigation
  or a current-state read. Retest the source-focus correction from PR #272 within
  the real capacity journey; its prior focused proof is supporting evidence.

### Screen reader

- Record announcements for document title, landmarks, headings, visible control
  labels, required fields, instructions, errors, review context and changed status.
- Check reading order and discoverability with normal reader commands/rotor.
  Confirm the accessible name contains the visible action wording, including
  portal identity. Inspect notifications without requiring a pointer.
- Check review/correction/result focus and the announcement after an asynchronous
  save, lost connection, changed role, event or account. Old private payloads must
  disappear when current access is lost; do not treat old text as a valid receipt.

### Touch, zoom and themes

- On actual mobile browsers, open the virtual keyboard, enter/correct text, reach
  footer actions, dismiss overlays and navigate to the next action. Verify target
  usability, orientation and absence of pointer-only interactions.
- Use actual browser zoom and text resizing; record chosen percentages and CSS
  viewport. Check 200% text resize and 320 CSS-pixel reflow. A CSS transform or
  screenshot resize does not exercise browser zoom. Review sticky bars, tables,
  dialogs and nested guide previews for hidden controls or lost content.
- Review platform light/dark, event light/dark, threshold custom palette and
  system/forced-colour settings where supported. Verify focus/text/non-text
  contrast, errors and disabled/current states, including portalled overlays.

## Complete the standards review

Evaluate every row in `wcag22-aa-review.json` across the full pages/processes.
Classify each as passed, failed, not evaluated or inapplicable with a reason.
Do not infer a manual pass from a scanner's empty violation list. Include media,
time limits, authentication, redundant entry, help consistency, input alternatives
and dynamic semantics when deciding applicability.

A failed critical finding needs a reproducible case, bounded correction and an
actual retest on the same affected journey/environment. Record remaining issues;
keep the release gate open while required evidence is missing. Root owns issue
acceptance and release sign-off after the existing dependencies are complete.
