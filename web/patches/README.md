# OpenVue patches

## Dialog initial focus (`openvue@1.0.0.patch`)

OpenVue's Dialog forces `focusVisible: true` when moving focus into a newly
opened dialog. Browsers supporting that option show a focus ring even when the
dialog was opened with a pointer. The fallback target is the maximize button
or, for non-maximizable dialogs, the close button.

The patch removes only that option from the source and published module. The
browser decides focus visibility using its normal input-modality heuristics.
Initial focus, focus trapping, focus restoration, and keyboard focus styles are
unchanged. It applies to every Dialog, including ConfirmDialog, without CSS
overrides or per-modal handlers.

Bun applies this patch during installation. Docker must copy `patches/` before
`bun install`. Remove the patch and `patchedDependencies` entry once the
upstream package includes the fix.

Regression checks:

- Click to open/reopen a maximizable dialog and a regular dialog: the initially
  focused header button should not acquire a forced ring.
- Tab to a trigger and open with Enter/Space: initial keyboard focus remains
  visible. Tab/Shift+Tab remain trapped inside the dialog.
- Escape closes the dialog and returns focus to its trigger.
- Explicitly autofocus an input: it still receives focus.

`DialogFocus.spec.ts` checks the installed module's initial-focus behavior;
input-modality and rendered focus rings require a real browser.
