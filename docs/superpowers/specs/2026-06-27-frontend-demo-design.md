# GoJudge Frontend Demo Design

## Summary

Build an anonymous frontend demo for the existing GoJudge backend. The frontend must let a reviewer browse problems, read a problem, edit Go/C++/Python code, submit it, observe asynchronous judge progress, and inspect submission history without exposing hidden test cases or moving code execution into the frontend or API process.

The selected visual direction is a competition-oriented split workbench: navy navigation, yellow brand accents, a red primary submit action, and green judge success states. The product remains an operational tool rather than a marketing landing page.

## Goals

- Provide a complete browser flow over the existing MVP API.
- Preserve the backend's API/worker/Docker isolation boundary.
- Present problem text, code editor, and judge result in one efficient desktop workspace.
- Remain usable on mobile through tabs rather than compressed columns.
- Run through Docker Compose and be understandable to GitHub reviewers.

## Non-Goals

- User accounts, authentication, personal ownership, or permissions.
- Contest creation, leaderboards, or administrator workflows.
- SSE or WebSocket delivery; this slice uses polling.
- Editing problems or hidden test cases.
- Adding business logic to the Next.js proxy.

## Technical Approach

Use Next.js App Router with TypeScript under `frontend/`. Browser requests target same-origin `/api/*` endpoints. A catch-all Next.js Route Handler forwards method, body, query string, status, and response payload to the Go API through `API_INTERNAL_URL`.

This avoids browser CORS configuration and keeps deployment addresses outside client bundles. Monaco loads through a client-only dynamic import. Native fetch and focused React hooks are sufficient; no global state framework or request library is required for this scope.

## Routes

- `/`: fetch the problem list and redirect to the first available problem. If no problems exist, render the empty workbench state.
- `/problems/[id]`: primary split workbench for the selected problem.
- `/submissions`: full submission history ordered by the backend response.
- `/api/[...path]`: transparent proxy to the Go API.

## Workbench Layout

### Desktop

- Top bar: GoJudge brand, Problems and Submissions navigation, backend availability indicator.
- Left rail: problem list and compact recent-submission list.
- Center pane: title, limits, description, input/output details, and examples.
- Right pane: language selector, Monaco editor, submit action, and local draft state.
- Bottom result panel: latest submission state, duration, output, stderr, and retry action where applicable.

The panes use stable grid constraints and independently scroll when content grows. Controls do not resize while loading.

### Mobile

The same workbench becomes three tabs: Problem, Code, and Result. The header and active tab remain visible. The editor receives a stable minimum height, and the submit action stays in the Code tab.

## Visual System

- Navy provides navigation and structural emphasis without covering the entire UI.
- White and cool gray surfaces carry dense reading and list content.
- Yellow marks the GoJudge brand and small competition accents.
- Red is reserved for the primary Submit action and destructive/error emphasis.
- Green is reserved for Accepted and healthy service states.
- Wrong Answer uses amber; Runtime Error, Time Limit Exceeded, and Internal Error use red.
- Cards are used only for repeated submission entries on narrow layouts; desktop sections remain unframed panes.
- Lucide icons are used for familiar actions and status affordances.

## Data Flow

1. The workbench fetches `/api/problems` and `/api/problems/{id}`.
2. The editor loads the draft keyed by problem ID and language from local storage, falling back to a language starter template.
3. A submit action posts `{problemId, language, code}` to `/api/submissions`.
4. The returned queued submission becomes the active result.
5. The client polls `/api/submissions/{id}` once per second while status is `queued` or `running`.
6. Polling stops for all terminal statuses or when the component unmounts.
7. Recent and full history views fetch `/api/submissions` after a successful submission and on navigation.

Changing language selects that language's independent draft. User code is never cleared because of a request or polling error.

## Component Boundaries

- `AppShell`: top navigation and service state.
- `ProblemRail`: problem navigation and recent submissions.
- `ProblemStatement`: public problem fields only.
- `CodeWorkspace`: language selection, dynamic Monaco editor, draft persistence, and submit action.
- `JudgeResultPanel`: queued/running/terminal result rendering.
- `SubmissionHistory`: accessible history table with responsive compact rows.
- `StatusBadge`: canonical label, color, and icon mapping.
- `useSubmissionPolling`: abortable polling lifecycle with terminal-state detection.
- `api`: typed fetch helpers and structured error normalization.

Each component accepts data through typed props and does not know the internal Go API address.

## Loading, Empty, and Error Behavior

- Problem and submission requests render stable skeletons without shifting pane dimensions.
- An empty problem list renders a neutral empty workbench rather than redirecting repeatedly.
- Unknown problem IDs render a dedicated Not Found state.
- Proxy and backend errors preserve the HTTP status and structured error message.
- Failed submissions retain the editor content and expose a Retry action.
- Poll failures stop the active interval, retain the last known status, and expose a retry control.
- Abort errors caused by navigation or unmount are silent.
- Backend availability is informative; it does not block reading already loaded content.

## Testing Strategy

### Unit and Component Tests

Use Vitest and React Testing Library to cover:

- Canonical status labels and visual variants.
- Starter templates and problem/language draft keys.
- Draft persistence without overwriting existing user code.
- Submission request success and structured API errors.
- Polling continuation for queued/running states.
- Polling termination for every terminal state and component unmount.
- Result and history rendering for Accepted, Wrong Answer, Runtime Error, Time Limit Exceeded, and Internal Error.
- Mobile tab behavior.

### Browser Tests

Use Playwright against the Compose stack to verify:

- Initial problem selection and navigation.
- Go, C++, and Python submission flows.
- Queued/running progression to a terminal result.
- Submission history refresh and navigation.
- Draft restoration after language and problem switches.
- Desktop split-pane and mobile tab layouts without overlap.

## Deployment

Add a multi-stage frontend Dockerfile and a `frontend` Compose service. The service receives `API_INTERNAL_URL=http://api:8080`, depends on the API, exposes host port `3000`, and includes a health check. The browser never receives the internal service URL.

CI adds frontend dependency installation, linting, type checking, unit tests, and a production build while retaining the existing Go checks. README documents `http://localhost:3000`, the browser workflow, architecture, and screenshots.

## Acceptance Criteria

- `make compose-up` starts the frontend with the existing API, worker, PostgreSQL, Redis, and MinIO services.
- A reviewer can complete the anonymous browse-edit-submit-result-history flow in a browser.
- Go, C++, and Python are selectable and retain independent drafts.
- Polling stops on terminal results and on navigation.
- Desktop and mobile layouts remain usable without overlapping content.
- Frontend lint, type checking, unit tests, production build, and browser tests pass.
- Existing Go tests and backend behavior remain unchanged.
