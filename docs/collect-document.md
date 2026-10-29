# Collect a document (Core)

Administrators can start from an empty client directory or empty file view, or use **Help → Getting started: Collect a document** at any time. The floating OpenVue Panel contains instructions and a vertical Stepper and is collapsible and dismissible. All client selection, navigation, creation, and copying happen through the app’s real controls, not duplicate actions inside the panel. Collection links default to Write and email unchecked; configured expiry defaults and existing validation remain in use.

After explicit start or continuation, amber spotlight hints identify the relevant control, with dimming and a directional arrow. The route is Clients → client name → Secure Links → New Link → Copy, including the mobile navigation menu. Opening a client from the directory selects it for an active tour; copying an existing active upload link adopts it for the request. Progress advances on successful app actions, not clicking a tour “Next” button. Escape dismisses a hint without advancing progress or closing its dialog. Typing or clicking continues the real action; the dimming layer does not intercept clicks. Guidance is session-only and does not replay on restored progress. The checklist temporarily minimizes and moves away if it would cover the highlighted control; it expands on the next workflow step. Coach placement accommodates narrow viewports and stays within the dialog’s focus scope for form fields.

The interaction combines the task checklist and contextual guidance patterns described by [Appcues](https://www.appcues.com/blog/user-onboarding-ui-ux-patterns) and [Fullstory](https://www.fullstory.com/blog/user-onboarding-checklist), using the documented [OpenVue Stepper](https://openvue.dev/stepper) and Panel APIs.

On mobile, only the current step is shown by default; **All steps** expands the overview. During a spotlight, the panel temporarily shrinks to its header without saving a collapsed state. Callouts keep 44px close targets and prefer placement above form fields to leave the following controls accessible. Input events, including paste and autofill-style input, dismiss the field hint.

Callouts use the action as their heading. Selecting the current Stepper item restores its contextual highlight without advancing or resetting progress; Enter and Space work too. Inactive steps stay disabled. Dismissing a replayed hint restores focus to the step, including after the mobile panel reopens. In the workspace’s final step, replay points to the Files heading.

A first-time administrator signing in with no clients lands in Clients and sees the empty-state invitation. No tour or spotlight appears until they choose **Collect a document**. The tour then highlights **New Client**, its name field, and the first client’s directory link after successful creation.

## State and evidence

- Per-user, origin/tenant-scoped browser storage contains the start time, client/link IDs, collapse/dismiss state, and successful copy or email-queue acceptance. No recipient address, token, or link URL is stored there.
- Client and link validity are refreshed from authorized APIs on navigation, return to the window, relevant successful store actions, page reload, and cross-tab storage changes. A one-shot expiry timer invalidates an open checklist; there is no polling loop.
- Existing active links can be explicitly selected. Newly created links for the selected client are reconciled from server state. Restart resets only checklist context.
- Copy means **copied**, not sent. `email_queued` means accepted into the application's send queue, not transport acceptance or delivery. Sender preparation and document exchange remain separate.
- Receipt requires an upload activity for the selected tenant, client, and secure-link ID after the checklist's client-selection/start timestamp and link creation. Pre-existing files and authenticated staff uploads do not count.
- Uploads now record the authenticated secure-link ID. Older uploads missing that attribution cannot establish completion. A link identifies the access mechanism, not a particular person: the UI says **A file was uploaded through this link.** Anyone holding that link could have uploaded it. Uploads made via a separate client account do not establish this link's completion.
- A revoked link is removed from derived progress; an expired link needs replacement unless receipt was already established. Deleted clients return to client selection. Browser-storage failure permits session-only use.

## Authorization and guide integration

Link management and the token-free collection status endpoint require an administrator. Managers and clients do not receive this administrator workflow. During verification, missing file-API enforcement was found and fixed: current stored link permissions/expiry/revocation now govern file access, and Write links cannot read/download/delete files. Folder navigation remains available for uploading.

Topic URLs resolve under the configured Help guide URL (branding URL fallback), including bundled `/docs/` and external guide bases. Links cover first exchange, client management, creating links, requesting documents, and activity. Help destinations remain capability-driven. Core has no billing/support/calendar dependency.

There is no existing product analytics abstraction. No analytics service or conversion events were added, and guide/hint views are not counted as activation. Consequently sample/demo uploads are not emitted as conversion events.

## Verification

- `go test ./internal/handlers ./internal/services`: status scoping, old/staff/wrong-client activity exclusions, authenticated upload attribution, manager/client denial, permission changes, expiry/revocation, deletion, and queue acceptance.
- `bun run --bun test:unit --run`: frontend suite, including reconciliation, stale links, permission changes, resume, and bundled/external URL resolution.
- Typecheck, production frontend build, targeted ESLint and Prettier checks.
- `collection_browser.py`: real API/browser login with a run-specific administrator in the copied preview DB; existing client/link forms, successful clipboard copy, reload/resume, cross-tab dismiss/resume, pre-existing and subsequent staff uploads, anonymous recipient upload, actual receipt, guide response, revocation of an open session, deletion of the run-specific client, mobile bounds, keyboard hints, and reduced motion.
- `collection_ui_browser.py`: sign-in and first-client creation with mocked empty-account APIs, compact/current-step and full-overview mobile modes, collapsed/dismissed resume, automatic field hints, manager/client visibility, hosted/self-hosted Help capabilities, bundled/external guide URLs at 1280px, 390px, and 320px widths. These capability configurations are UI fixtures, not separate deployed environments.

Browser scripts use isolated fixtures, never reset shared data, and require outbound email to be disabled. Screenshots stay in ignored `data/`.
