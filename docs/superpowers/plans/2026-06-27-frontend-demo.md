# GoJudge Frontend Demo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an anonymous Next.js frontend that lets a reviewer browse problems, write Go/C++/Python code, submit it, observe judge status, and inspect submission history.

**Architecture:** A TypeScript Next.js App Router application lives in `frontend/`. Browser requests use a same-origin catch-all Route Handler that transparently proxies to the existing Go API through `API_INTERNAL_URL`; Monaco stays client-only, and an abortable hook polls queued/running submissions until terminal status.

**Tech Stack:** Next.js App Router, React, TypeScript, `@monaco-editor/react`, Lucide React, Vitest, React Testing Library, Playwright, Docker Compose.

---

## File Structure

Create these focused units:

```text
frontend/
  app/
    api/[...path]/route.ts       same-origin Go API proxy
    problems/[id]/page.tsx       selected-problem route
    submissions/page.tsx         full history route
    globals.css                  competition-workbench tokens and responsive layout
    layout.tsx                   root metadata and AppShell
    page.tsx                     first-problem redirect / empty state
  components/
    app-shell.tsx                global navigation and service indicator
    code-workspace.tsx           language, Monaco, drafts, submit action
    judge-result-panel.tsx       active result and retry state
    problem-rail.tsx             problem navigation and recent submissions
    problem-statement.tsx        public problem presentation
    status-badge.tsx             canonical status presentation
    submission-history.tsx       responsive history table/list
    workbench.tsx                data orchestration and pane composition
  hooks/
    use-local-draft.ts           per-problem/language local storage
    use-submission-polling.ts    abortable terminal-aware polling
  lib/
    api.ts                       typed fetch helpers and error normalization
    judge.ts                     status metadata, templates, draft keys
    types.ts                     API contract types
  tests/                         Vitest component and hook tests
  e2e/                           Playwright browser tests
  Dockerfile                     production standalone Next.js image
  package.json
  playwright.config.ts
  vitest.config.ts
docker-compose.yml               add frontend and API health checks
Makefile                         frontend test/build targets
.github/workflows/ci.yml         frontend verification job
README.md                        browser quick start and screenshots
```

### Task 1: Scaffold the Next.js Testable Frontend

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/tsconfig.json`
- Create: `frontend/next.config.ts`
- Create: `frontend/eslint.config.mjs`
- Create: `frontend/vitest.config.ts`
- Create: `frontend/tests/setup.ts`
- Create: `frontend/app/layout.tsx`
- Create: `frontend/app/page.tsx`
- Modify: `.gitignore`

- [ ] **Step 1: Add generated-content ignores before installing dependencies**

Append these exact entries to `.gitignore`:

```gitignore
.superpowers/
frontend/.next/
frontend/node_modules/
frontend/playwright-report/
frontend/test-results/
frontend/coverage/
```

- [ ] **Step 2: Scaffold Next.js and install runtime/test dependencies**

Run:

```bash
npx create-next-app@latest frontend --ts --eslint --app --src-dir=false --use-npm --no-tailwind --import-alias='@/*'
cd frontend
npm install @monaco-editor/react lucide-react
npm install --save-dev vitest jsdom @testing-library/react @testing-library/jest-dom @testing-library/user-event @vitejs/plugin-react playwright @playwright/test
```

Expected: `frontend/package-lock.json` exists and `npm install` exits 0.

- [ ] **Step 3: Add deterministic verification scripts**

Set `frontend/package.json` scripts to:

```json
{
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "start": "next start",
    "lint": "eslint .",
    "typecheck": "tsc --noEmit",
    "test": "vitest",
    "test:run": "vitest run --passWithNoTests",
    "test:e2e": "playwright test"
  }
}
```

- [ ] **Step 4: Configure Vitest and DOM matchers**

Create `frontend/vitest.config.ts`:

```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./tests/setup.ts"],
    restoreMocks: true,
  },
  resolve: {
    alias: { "@": path.resolve(__dirname, ".") },
  },
});
```

Create `frontend/tests/setup.ts`:

```ts
import "@testing-library/jest-dom/vitest";
```

- [ ] **Step 5: Run the empty-project verification**

Run:

```bash
cd frontend
npm run lint
npm run typecheck
npm run test:run
npm run build
```

Expected: all commands exit 0; Vitest reports no test files and exits 0 because `--passWithNoTests` is explicit.

- [ ] **Step 6: Commit the scaffold**

```bash
git add .gitignore frontend
git commit -m "chore: scaffold frontend application"
```

### Task 2: Define the API Contract and Same-Origin Proxy

**Files:**
- Create: `frontend/lib/types.ts`
- Create: `frontend/lib/api.ts`
- Create: `frontend/tests/api.test.ts`
- Create: `frontend/app/api/[...path]/route.ts`
- Create: `frontend/tests/proxy.test.ts`

- [ ] **Step 1: Write failing typed-fetch tests**

Create `frontend/tests/api.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, getProblems } from "@/lib/api";

afterEach(() => vi.unstubAllGlobals());

describe("getProblems", () => {
  it("returns typed problems", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify([
      { id: "sum", title: "A+B", description: "Add", language: "go", timeLimitMs: 1000, memoryLimitMb: 64 },
    ]), { status: 200 })));

    await expect(getProblems()).resolves.toMatchObject([{ id: "sum", title: "A+B" }]);
  });

  it("normalizes structured API errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "backend_unavailable", message: "service unavailable" },
    }), { status: 503 })));

    await expect(getProblems()).rejects.toEqual(
      new ApiError(503, "backend_unavailable", "service unavailable"),
    );
  });
});
```

- [ ] **Step 2: Run the test to verify RED**

Run: `cd frontend && npm run test:run -- tests/api.test.ts`

Expected: FAIL because `@/lib/api` does not exist.

- [ ] **Step 3: Add exact API types and helpers**

Create `frontend/lib/types.ts` with these exported types:

```ts
export type Language = "go" | "cpp" | "python";
export type SubmissionStatus =
  | "queued" | "running" | "accepted" | "wrong_answer"
  | "runtime_error" | "time_limit_exceeded" | "internal_error";

export interface Problem {
  id: string;
  title: string;
  description: string;
  language: Language;
  timeLimitMs: number;
  memoryLimitMb: number;
}

export interface JudgeResult {
  status: SubmissionStatus;
  stdout?: string;
  stderr?: string;
  exitCode?: number;
  durationMs?: number;
}

export interface Submission {
  id: string;
  problemId: string;
  language: Language;
  status: SubmissionStatus;
  result?: JudgeResult;
  createdAt: string;
  updatedAt: string;
}

export interface CreateSubmissionInput {
  problemId: string;
  language: Language;
  code: string;
}
```

Create `frontend/lib/api.ts` around this private `request<T>` helper, then add the exported endpoint functions below it:

```ts
import type { CreateSubmissionInput, Problem, Submission } from "@/lib/types";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    const body = payload as { error?: { code?: string; message?: string } } | null;
    throw new ApiError(
      response.status,
      body?.error?.code ?? "request_failed",
      body?.error?.message ?? `Request failed with status ${response.status}`,
    );
  }
  return payload as T;
}
```

Export:

```ts
export const getProblems = () => request<Problem[]>("/api/problems");
export const getProblem = (id: string) => request<Problem>(`/api/problems/${encodeURIComponent(id)}`);
export const getSubmissions = () => request<Submission[]>("/api/submissions");
export const getSubmission = (id: string, signal?: AbortSignal) =>
  request<Submission>(`/api/submissions/${encodeURIComponent(id)}`, { signal });
export const createSubmission = (input: CreateSubmissionInput) =>
  request<Submission>("/api/submissions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
```

- [ ] **Step 4: Verify typed helpers are GREEN**

Run: `cd frontend && npm run test:run -- tests/api.test.ts`

Expected: PASS.

- [ ] **Step 5: Write a failing proxy URL test**

Create `frontend/tests/proxy.test.ts`:

```ts
import { expect, it } from "vitest";
import { backendURL } from "@/app/api/[...path]/route";

it("preserves path and query without exposing the internal URL", () => {
  expect(backendURL(["submissions", "sub-1"], "?verbose=1", "http://api:8080"))
    .toBe("http://api:8080/submissions/sub-1?verbose=1");
});
```

- [ ] **Step 6: Run proxy test to verify RED**

Run: `cd frontend && npm run test:run -- tests/proxy.test.ts`

Expected: FAIL because the Route Handler does not exist.

- [ ] **Step 7: Implement the transparent Route Handler**

Create `frontend/app/api/[...path]/route.ts`. Export `backendURL` and one `proxy` function. `GET` and `POST` call `proxy`; forwarded headers include `content-type`, request bodies are omitted for GET/HEAD, and the response preserves status, content type, and body bytes. Reject a missing `API_INTERNAL_URL` with JSON status 500.

Use this URL builder exactly:

```ts
export function backendURL(path: string[], search: string, base = process.env.API_INTERNAL_URL ?? "") {
  if (!base) throw new Error("API_INTERNAL_URL is not configured");
  return `${base.replace(/\/$/, "")}/${path.map(encodeURIComponent).join("/")}${search}`;
}
```

- [ ] **Step 8: Run API/proxy tests and commit**

Run: `cd frontend && npm run test:run -- tests/api.test.ts tests/proxy.test.ts`

Expected: PASS.

```bash
git add frontend/app/api frontend/lib frontend/tests/api.test.ts frontend/tests/proxy.test.ts
git commit -m "feat: add frontend API proxy"
```

### Task 3: Implement Judge Metadata, Templates, and Draft Persistence

**Files:**
- Create: `frontend/lib/judge.ts`
- Create: `frontend/hooks/use-local-draft.ts`
- Create: `frontend/tests/judge.test.ts`
- Create: `frontend/tests/use-local-draft.test.tsx`

- [ ] **Step 1: Write failing judge-domain tests**

Create `frontend/tests/judge.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { draftKey, isTerminalStatus, starterTemplate } from "@/lib/judge";

describe("judge helpers", () => {
  it.each(["accepted", "wrong_answer", "runtime_error", "time_limit_exceeded", "internal_error"] as const)(
    "treats %s as terminal", status => expect(isTerminalStatus(status)).toBe(true),
  );
  it.each(["queued", "running"] as const)(
    "keeps polling %s", status => expect(isTerminalStatus(status)).toBe(false),
  );
  it("uses a problem/language-specific draft key", () => {
    expect(draftKey("sum", "python")).toBe("gojudge:draft:sum:python");
  });
  it("provides runnable starter templates", () => {
    expect(starterTemplate("go")).toContain("package main");
    expect(starterTemplate("cpp")).toContain("#include <iostream>");
    expect(starterTemplate("python")).toContain("def main");
  });
});
```

- [ ] **Step 2: Verify RED, implement, verify GREEN**

Run: `cd frontend && npm run test:run -- tests/judge.test.ts`

Expected RED: module missing.

Create `frontend/lib/judge.ts` with exhaustive status metadata plus these helpers:

```ts
import type { Language, SubmissionStatus } from "@/lib/types";

const terminalStatuses = new Set<SubmissionStatus>([
  "accepted", "wrong_answer", "runtime_error", "time_limit_exceeded", "internal_error",
]);

export const isTerminalStatus = (status: SubmissionStatus) => terminalStatuses.has(status);
export const draftKey = (problemId: string, language: Language) => `gojudge:draft:${problemId}:${language}`;

const templates: Record<Language, string> = {
  go: 'package main\n\nimport "fmt"\n\nfunc main() {\n\tvar a, b int\n\tfmt.Scan(&a, &b)\n\tfmt.Println(a + b)\n}\n',
  cpp: '#include <iostream>\n\nint main() {\n    long long a, b;\n    std::cin >> a >> b;\n    std::cout << a + b << "\\n";\n    return 0;\n}\n',
  python: 'def main():\n    a, b = map(int, input().split())\n    print(a + b)\n\nif __name__ == "__main__":\n    main()\n',
};

export const starterTemplate = (language: Language) => templates[language];
```

Also export a `statusMeta` record containing the canonical labels and visual variants consumed by `StatusBadge`. Run the same command; expected PASS.

- [ ] **Step 3: Write the failing local-draft hook test**

Create `frontend/tests/use-local-draft.test.tsx`:

```tsx
import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it } from "vitest";
import { useLocalDraft } from "@/hooks/use-local-draft";

beforeEach(() => localStorage.clear());

it("restores an existing draft without replacing it with a starter", () => {
  localStorage.setItem("gojudge:draft:sum:go", "package main // saved");
  const { result } = renderHook(() => useLocalDraft("sum", "go"));
  expect(result.current.code).toBe("package main // saved");
  act(() => result.current.setCode("package main // changed"));
  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("package main // changed");
});
```

- [ ] **Step 4: Verify RED, implement, verify GREEN**

Run: `cd frontend && npm run test:run -- tests/use-local-draft.test.tsx`

Implement `useLocalDraft(problemId, language)` with key-aware initialization so changing language never writes the old language's code into the new key:

```ts
"use client";

import { useEffect, useMemo, useState } from "react";
import type { Language } from "@/lib/types";
import { draftKey, starterTemplate } from "@/lib/judge";

export function useLocalDraft(problemId: string, language: Language) {
  const key = useMemo(() => draftKey(problemId, language), [problemId, language]);
  const [code, setCode] = useState(() => starterTemplate(language));
  const [readyKey, setReadyKey] = useState<string | null>(null);

  useEffect(() => {
    setCode(localStorage.getItem(key) ?? starterTemplate(language));
    setReadyKey(key);
  }, [key, language]);

  useEffect(() => {
    if (readyKey === key) localStorage.setItem(key, code);
  }, [code, key, readyKey]);

  return { code, setCode };
}
```

Re-run; expected PASS.

- [ ] **Step 5: Commit judge domain utilities**

```bash
git add frontend/lib/judge.ts frontend/hooks/use-local-draft.ts frontend/tests/judge.test.ts frontend/tests/use-local-draft.test.tsx
git commit -m "feat: add judge frontend domain helpers"
```

### Task 4: Build Abortable Submission Polling

**Files:**
- Create: `frontend/hooks/use-submission-polling.ts`
- Create: `frontend/tests/use-submission-polling.test.tsx`

- [ ] **Step 1: Write failing fake-timer tests**

Create tests that inject a `loadSubmission(id, signal)` function and assert:

```tsx
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useSubmissionPolling } from "@/hooks/use-submission-polling";
import type { Submission } from "@/lib/types";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

const queuedSubmission: Submission = {
  id: "sub-1",
  problemId: "sum",
  language: "go",
  status: "queued",
  createdAt: "2026-06-27T00:00:00Z",
  updatedAt: "2026-06-27T00:00:00Z",
};

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

it("polls queued submissions and stops at accepted", async () => {
  const load = vi.fn()
    .mockResolvedValueOnce(queuedSubmission)
    .mockResolvedValueOnce({ ...queuedSubmission, status: "running" })
    .mockResolvedValueOnce({ ...queuedSubmission, status: "accepted", result: { status: "accepted" } });
  const { result } = renderHook(() => useSubmissionPolling("sub-1", load, 1000));
  await advance(0);
  await advance(1000);
  await advance(1000);
  expect(result.current.submission?.status).toBe("accepted");
  expect(load).toHaveBeenCalledTimes(3);
  await vi.advanceTimersByTimeAsync(3000);
  expect(load).toHaveBeenCalledTimes(3);
});

it("aborts the active request on unmount", () => {
  const load = vi.fn((_id, signal) => new Promise((_resolve, reject) => {
    signal.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")));
  }));
  const { unmount } = renderHook(() => useSubmissionPolling("sub-1", load, 1000));
  unmount();
  expect(load.mock.calls[0][1].aborted).toBe(true);
});
```

- [ ] **Step 2: Run tests to verify RED**

Run: `cd frontend && npm run test:run -- tests/use-submission-polling.test.tsx`

Expected: FAIL because the hook is missing.

- [ ] **Step 3: Implement one-request-at-a-time polling**

Implement `frontend/hooks/use-submission-polling.ts`:

```ts
"use client";

import { useCallback, useEffect, useState } from "react";
import { getSubmission } from "@/lib/api";
import { isTerminalStatus } from "@/lib/judge";
import type { Submission } from "@/lib/types";

type Loader = (id: string, signal: AbortSignal) => Promise<Submission>;

export function useSubmissionPolling(id: string | null, load: Loader = getSubmission, intervalMs = 1000) {
  const [submission, setSubmission] = useState<Submission | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isPolling, setIsPolling] = useState(false);
  const [retryVersion, setRetryVersion] = useState(0);
  const retry = useCallback(() => {
    setError(null);
    setRetryVersion(version => version + 1);
  }, []);

  useEffect(() => {
    if (!id) {
      setSubmission(null);
      setIsPolling(false);
      return;
    }
    let stopped = false;
    let timeout: ReturnType<typeof setTimeout> | undefined;
    let controller: AbortController | undefined;
    setIsPolling(true);
    setError(null);

    const poll = async () => {
      controller = new AbortController();
      try {
        const next = await load(id, controller.signal);
        if (stopped) return;
        setSubmission(next);
        if (isTerminalStatus(next.status)) {
          setIsPolling(false);
        } else {
          timeout = setTimeout(poll, intervalMs);
        }
      } catch (caught) {
        if (stopped || (caught instanceof DOMException && caught.name === "AbortError")) return;
        setError(caught instanceof Error ? caught.message : "Polling failed");
        setIsPolling(false);
      }
    };
    void poll();
    return () => {
      stopped = true;
      if (timeout) clearTimeout(timeout);
      controller?.abort();
    };
  }, [id, intervalMs, load, retryVersion]);

  return { submission, error, isPolling, retry };
}
```

- [ ] **Step 4: Verify GREEN and commit**

Run: `cd frontend && npm run test:run -- tests/use-submission-polling.test.tsx`

Expected: PASS with no open-handle warning.

```bash
git add frontend/hooks/use-submission-polling.ts frontend/tests/use-submission-polling.test.tsx
git commit -m "feat: add submission polling hook"
```

### Task 5: Build the App Shell and Status Components

**Files:**
- Create: `frontend/components/app-shell.tsx`
- Create: `frontend/components/status-badge.tsx`
- Create: `frontend/tests/status-badge.test.tsx`
- Create: `frontend/tests/app-shell.test.tsx`
- Modify: `frontend/app/layout.tsx`
- Modify: `frontend/app/globals.css`

- [ ] **Step 1: Write a failing status presentation test**

```tsx
import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { StatusBadge } from "@/components/status-badge";

it("renders the canonical accepted label and status hook", () => {
  render(<StatusBadge status="accepted" />);
  expect(screen.getByText("Accepted")).toHaveAttribute("data-status", "accepted");
});
```

- [ ] **Step 2: Verify RED and implement `StatusBadge`**

Run: `cd frontend && npm run test:run -- tests/status-badge.test.tsx`

Expected RED: component missing.

Implement an exhaustive status-to-label/icon mapping using Lucide icons. Every badge uses `data-status`, visible text, and an icon with `aria-hidden="true"`. Re-run; expected PASS.

- [ ] **Step 3: Write a failing service-indicator test**

Create `frontend/tests/app-shell.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { AppShell } from "@/components/app-shell";

it("reports backend health without blocking navigation", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response('{"status":"ok"}', { status: 200 })));
  render(<AppShell><main>content</main></AppShell>);
  expect(screen.getByRole("link", { name: "Problems" })).toHaveAttribute("href", "/");
  expect(screen.getByRole("link", { name: "Submissions" })).toHaveAttribute("href", "/submissions");
  expect(await screen.findByText("Online")).toBeVisible();
  expect(screen.getByRole("main")).toHaveTextContent("content");
});
```

- [ ] **Step 4: Verify RED and implement the competition AppShell and global tokens**

Run: `cd frontend && npm run test:run -- tests/app-shell.test.tsx`

Expected RED: AppShell missing. Implement a client component that requests `/api/healthz` on mount, renders Online/Unavailable without hiding children, and aborts on unmount. Use CSS custom properties for navy, yellow, red, green, amber, white, cool grays, borders, and focus rings. Build a 48-56px top bar with text brand `GOJUDGE`, Problems/Submissions navigation, and a compact service indicator. Keep letter spacing at zero and all cards at 8px radius or less. Re-run the test; expected PASS.

- [ ] **Step 5: Run component and static checks**

Run:

```bash
cd frontend
npm run test:run -- tests/status-badge.test.tsx tests/app-shell.test.tsx
npm run lint
npm run typecheck
```

Expected: PASS.

- [ ] **Step 6: Commit shell and status UI**

```bash
git add frontend/app/layout.tsx frontend/app/globals.css frontend/components/app-shell.tsx frontend/components/status-badge.tsx frontend/tests/status-badge.test.tsx frontend/tests/app-shell.test.tsx
git commit -m "feat: add competition frontend shell"
```

### Task 6: Build Problem Navigation and Statement Views

**Files:**
- Create: `frontend/components/problem-rail.tsx`
- Create: `frontend/components/problem-statement.tsx`
- Create: `frontend/tests/problem-views.test.tsx`
- Modify: `frontend/app/page.tsx`

- [ ] **Step 1: Write failing public-problem rendering tests**

Create `frontend/tests/problem-views.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { ProblemRail } from "@/components/problem-rail";
import { ProblemStatement } from "@/components/problem-statement";
import type { Problem } from "@/lib/types";

const problems: Problem[] = [
  { id: "sum", title: "A+B", description: "Add two numbers", language: "go", timeLimitMs: 1000, memoryLimitMb: 64 },
  { id: "echo", title: "Echo", description: "Echo input", language: "go", timeLimitMs: 1000, memoryLimitMb: 64 },
];

it("links problems and marks the active problem", () => {
  render(<ProblemRail problems={problems} activeProblemId="sum" recentSubmissions={[]} />);
  expect(screen.getByRole("link", { name: "A+B" })).toHaveAttribute("href", "/problems/sum");
  expect(screen.getByRole("link", { name: "A+B" })).toHaveAttribute("aria-current", "page");
  expect(screen.getByRole("link", { name: "Echo" })).not.toHaveAttribute("aria-current");
});

it("renders only public problem fields", () => {
  render(<ProblemStatement problem={problems[0]} />);
  expect(screen.getByRole("heading", { name: "A+B" })).toBeVisible();
  expect(screen.getByText("Add two numbers")).toBeVisible();
  expect(screen.getByText(/1000 ms/i)).toBeVisible();
  expect(screen.queryByText(/testCases/i)).not.toBeInTheDocument();
});
```

- [ ] **Step 2: Verify RED, implement focused components, verify GREEN**

Run: `cd frontend && npm run test:run -- tests/problem-views.test.tsx`

Expected RED: components missing.

Implement `ProblemRail` and `ProblemStatement` as typed presentational components, then re-run; expected PASS.

- [ ] **Step 3: Implement root selection**

`app/page.tsx` performs a no-store server fetch to `${API_INTERNAL_URL}/problems`; redirect to `/problems/${problems[0].id}` when non-empty, render an empty state when empty, and render a backend-unavailable state when the request fails.

- [ ] **Step 4: Run checks and commit**

```bash
cd frontend
npm run test:run -- tests/problem-views.test.tsx
npm run lint
npm run typecheck
cd ..
git add frontend/app/page.tsx frontend/components/problem-rail.tsx frontend/components/problem-statement.tsx frontend/tests/problem-views.test.tsx
git commit -m "feat: add problem browsing views"
```

### Task 7: Add Monaco, Language Selection, and Drafts

**Files:**
- Create: `frontend/components/code-workspace.tsx`
- Create: `frontend/tests/code-workspace.test.tsx`

- [ ] **Step 1: Write failing editor interaction tests**

Create `frontend/tests/code-workspace.test.tsx`:

```tsx
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { CodeWorkspace } from "@/components/code-workspace";

vi.mock("@monaco-editor/react", () => ({
  default: ({ value, onChange }: { value?: string; onChange?: (value?: string) => void }) => (
    <textarea aria-label="Code editor" value={value} onChange={event => onChange?.(event.target.value)} />
  ),
}));

beforeEach(() => localStorage.clear());

it("keeps independent language drafts and submits the active code", async () => {
  const user = userEvent.setup();
  const onSubmit = vi.fn().mockResolvedValue(undefined);
  render(<CodeWorkspace problemId="sum" submitting={false} onSubmit={onSubmit} />);

  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: "package main // saved" } });
  await user.selectOptions(screen.getByLabelText("Language"), "python");
  expect(screen.getByLabelText("Code editor")).toHaveValue(expect.stringContaining("def main"));
  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: "print(3)" } });
  await user.selectOptions(screen.getByLabelText("Language"), "go");
  expect(screen.getByLabelText("Code editor")).toHaveValue("package main // saved");

  await user.click(screen.getByRole("button", { name: "Submit" }));
  expect(onSubmit).toHaveBeenCalledWith({ language: "go", code: "package main // saved" });
  expect(screen.getByLabelText("Code editor")).toHaveValue("package main // saved");
});
```

- [ ] **Step 2: Verify RED**

Run: `cd frontend && npm run test:run -- tests/code-workspace.test.tsx`

Expected: FAIL because `CodeWorkspace` is missing.

- [ ] **Step 3: Implement the client-only editor**

Create a `"use client"` component. Load Monaco with:

```tsx
const MonacoEditor = dynamic(() => import("@monaco-editor/react"), {
  ssr: false,
  loading: () => <div className="editor-skeleton" aria-label="Loading code editor" />,
});
```

Use a compact native select for Go/C++/Python, `useLocalDraft`, a stable editor height, Monaco's `vs-dark` theme, and a red Submit button with a Lucide Play icon. Disable submission only while the POST is active or code is blank.

- [ ] **Step 4: Verify GREEN and commit**

```bash
cd frontend
npm run test:run -- tests/code-workspace.test.tsx
npm run lint
npm run typecheck
cd ..
git add frontend/components/code-workspace.tsx frontend/tests/code-workspace.test.tsx
git commit -m "feat: add Monaco code workspace"
```

### Task 8: Integrate the Workbench Submission Flow

**Files:**
- Create: `frontend/components/judge-result-panel.tsx`
- Create: `frontend/components/workbench.tsx`
- Create: `frontend/tests/judge-result-panel.test.tsx`
- Create: `frontend/tests/workbench.test.tsx`
- Create: `frontend/app/problems/[id]/page.tsx`

- [ ] **Step 1: Write failing result-state tests**

Create `frontend/tests/judge-result-panel.test.tsx` with a table test for all states:

```tsx
import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { JudgeResultPanel } from "@/components/judge-result-panel";
import type { Submission } from "@/lib/types";

const base: Submission = {
  id: "sub-1", problemId: "sum", language: "python", status: "queued",
  createdAt: "2026-06-27T00:00:00Z", updatedAt: "2026-06-27T00:00:00Z",
};

it.each([
  ["queued", "Queued"],
  ["running", "Running"],
  ["accepted", "Accepted"],
  ["wrong_answer", "Wrong Answer"],
  ["runtime_error", "Runtime Error"],
  ["time_limit_exceeded", "Time Limit Exceeded"],
  ["internal_error", "Internal Error"],
] as const)("renders %s", (status, label) => {
  render(<JudgeResultPanel submission={{ ...base, status, result: { status, stdout: "3\n", stderr: "boom", exitCode: 1, durationMs: 384 } }} error={null} onRetry={vi.fn()} />);
  expect(screen.getByText(label)).toBeVisible();
});

it("shows polling failure without discarding the last submission", () => {
  render(<JudgeResultPanel submission={base} error="network unavailable" onRetry={vi.fn()} />);
  expect(screen.getByText("Queued")).toBeVisible();
  expect(screen.getByRole("button", { name: "Retry" })).toBeVisible();
});
```

- [ ] **Step 2: Verify RED, implement `JudgeResultPanel`, verify GREEN**

Run: `cd frontend && npm run test:run -- tests/judge-result-panel.test.tsx`

Expected RED: component missing. Implement it with `StatusBadge`, stable result dimensions, and Retry only when `error` exists. Re-run; expected PASS.

- [ ] **Step 3: Write a failing workbench orchestration test**

Create `frontend/tests/workbench.test.tsx`. Mock API helpers, the editor, and polling at module boundaries:

```tsx
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { Workbench } from "@/components/workbench";

const mocks = vi.hoisted(() => ({
  getProblems: vi.fn(), getProblem: vi.fn(), getSubmissions: vi.fn(), createSubmission: vi.fn(),
}));
vi.mock("@/lib/api", () => mocks);
vi.mock("@/components/code-workspace", () => ({
  CodeWorkspace: ({ onSubmit }: { onSubmit: (input: { language: "python"; code: string }) => Promise<void> }) =>
    <button onClick={() => onSubmit({ language: "python", code: "print(3)" })}>Submit</button>,
}));
vi.mock("@/hooks/use-submission-polling", () => ({
  useSubmissionPolling: (id: string | null) => ({
    submission: id ? { id, problemId: "sum", language: "python", status: "accepted", result: { status: "accepted" }, createdAt: "x", updatedAt: "x" } : null,
    error: null, isPolling: false, retry: vi.fn(),
  }),
}));

it("loads the workspace, submits code, and refreshes history at terminal status", async () => {
  const problem = { id: "sum", title: "A+B", description: "Add", language: "go", timeLimitMs: 1000, memoryLimitMb: 64 };
  const queued = { id: "sub-1", problemId: "sum", language: "python", status: "queued", createdAt: "x", updatedAt: "x" };
  mocks.getProblems.mockResolvedValue([problem]);
  mocks.getProblem.mockResolvedValue(problem);
  mocks.getSubmissions.mockResolvedValue([]);
  mocks.createSubmission.mockResolvedValue(queued);

  render(<Workbench problemId="sum" />);
  await screen.findByRole("heading", { name: "A+B" });
  await userEvent.click(screen.getByRole("button", { name: "Submit" }));
  expect(mocks.createSubmission).toHaveBeenCalledWith({ problemId: "sum", language: "python", code: "print(3)" });
  await waitFor(() => expect(mocks.getSubmissions).toHaveBeenCalledTimes(2));
});
```

- [ ] **Step 4: Verify RED and implement `Workbench`**

Run: `cd frontend && npm run test:run -- tests/workbench.test.tsx`

Expected RED: component missing.

Implement one `"use client"` orchestration component that owns fetched data, active submission ID, submit error, mobile active tab, and refresh callbacks. Compose `ProblemRail`, `ProblemStatement`, `CodeWorkspace`, and `JudgeResultPanel`; do not duplicate their rendering logic. Create `app/problems/[id]/page.tsx` as a thin server route that resolves `params` and renders `<Workbench problemId={id} />`.

- [ ] **Step 5: Verify GREEN and commit**

```bash
cd frontend
npm run test:run -- tests/judge-result-panel.test.tsx tests/workbench.test.tsx
npm run lint
npm run typecheck
cd ..
git add frontend/app/problems frontend/components/judge-result-panel.tsx frontend/components/workbench.tsx frontend/tests/judge-result-panel.test.tsx frontend/tests/workbench.test.tsx
git commit -m "feat: integrate judge workbench"
```

### Task 9: Add Submission History

**Files:**
- Create: `frontend/components/submission-history.tsx`
- Create: `frontend/tests/submission-history.test.tsx`
- Create: `frontend/app/submissions/page.tsx`

- [ ] **Step 1: Write failing responsive-history tests**

Create `frontend/tests/submission-history.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { SubmissionHistory } from "@/components/submission-history";

const submissions = [
  { id: "sub-2", problemId: "echo", language: "python", status: "accepted", createdAt: "2026-06-27T02:00:00Z", updatedAt: "2026-06-27T02:00:01Z" },
  { id: "sub-1", problemId: "sum", language: "go", status: "wrong_answer", createdAt: "2026-06-27T01:00:00Z", updatedAt: "2026-06-27T01:00:01Z" },
] as const;

it("renders history in supplied newest-first order", () => {
  render(<SubmissionHistory submissions={[...submissions]} loading={false} error={null} onRetry={vi.fn()} />);
  const rows = screen.getAllByRole("row");
  expect(rows[1]).toHaveTextContent("echo");
  expect(rows[2]).toHaveTextContent("sum");
  expect(screen.getByRole("link", { name: "echo" })).toHaveAttribute("href", "/problems/echo");
});

it("renders empty and retry states", () => {
  const { rerender } = render(<SubmissionHistory submissions={[]} loading={false} error={null} onRetry={vi.fn()} />);
  expect(screen.getByText("No submissions yet")).toBeVisible();
  rerender(<SubmissionHistory submissions={[]} loading={false} error="network unavailable" onRetry={vi.fn()} />);
  expect(screen.getByRole("button", { name: "Retry" })).toBeVisible();
});
```

- [ ] **Step 2: Verify RED, implement, verify GREEN**

Run: `cd frontend && npm run test:run -- tests/submission-history.test.tsx`

Expected RED: component missing.

Implement semantic desktop `<table>` markup and CSS-driven compact mobile rows without duplicating fetched data. The page fetches through `/api/submissions` and supplies retry state.

- [ ] **Step 3: Run checks and commit**

```bash
cd frontend
npm run test:run -- tests/submission-history.test.tsx
npm run lint
npm run typecheck
cd ..
git add frontend/app/submissions frontend/components/submission-history.tsx frontend/tests/submission-history.test.tsx
git commit -m "feat: add submission history page"
```

### Task 10: Finish Responsive Styling and Accessibility

**Files:**
- Modify: `frontend/app/globals.css`
- Modify: `frontend/components/workbench.tsx`
- Create: `frontend/components/workbench-tabs.tsx`
- Create: `frontend/tests/mobile-tabs.test.tsx`

- [ ] **Step 1: Write failing mobile-tab tests**

Create `frontend/tests/mobile-tabs.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { WorkbenchTabs } from "@/components/workbench-tabs";

it("exposes accessible tabs and reports the selected pane", async () => {
  const onChange = vi.fn();
  const { rerender } = render(<WorkbenchTabs active="problem" onChange={onChange} />);
  expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute("aria-selected", "true");
  await userEvent.click(screen.getByRole("tab", { name: "Code" }));
  expect(onChange).toHaveBeenCalledWith("code");

  rerender(<WorkbenchTabs active="code" onChange={onChange} />);
  expect(screen.getByRole("tab", { name: "Code" })).toHaveAttribute("aria-selected", "true");
  expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute("aria-selected", "false");
});
```

- [ ] **Step 2: Verify RED, implement tab semantics, verify GREEN**

Run: `cd frontend && npm run test:run -- tests/mobile-tabs.test.tsx`

Expected RED: component missing. Implement `WorkbenchTabs` with the exact `"problem" | "code" | "result"` union, `role="tablist"`, stable button dimensions, `aria-controls`, and `aria-selected`. Workbench assigns matching panel IDs and `data-active` values. Re-run; expected PASS.

- [ ] **Step 3: Complete responsive CSS constraints**

Desktop uses explicit tracks such as `minmax(180px, 240px) minmax(320px, 0.9fr) minmax(420px, 1.1fr)`. Add independent pane scrolling, stable result height, visible focus rings, 44px mobile targets, no negative letter spacing, and no viewport-based font scaling. Below 900px, hide inactive panels and show the tab list.

- [ ] **Step 4: Run all frontend unit checks and commit**

```bash
cd frontend
npm run test:run
npm run lint
npm run typecheck
cd ..
git add frontend/app/globals.css frontend/components/workbench.tsx frontend/components/workbench-tabs.tsx frontend/tests/mobile-tabs.test.tsx
git commit -m "feat: polish responsive judge workspace"
```

### Task 11: Add Production Containers and CI

**Files:**
- Create: `frontend/Dockerfile`
- Create: `frontend/.dockerignore`
- Modify: `frontend/next.config.ts`
- Modify: `docker-compose.yml`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Verify current Compose has no frontend**

Run: `docker compose config --services`

Expected: output does not contain `frontend`.

- [ ] **Step 2: Add a standalone production image**

Set `output: "standalone"` in `next.config.ts`. Create a multi-stage Node Alpine Dockerfile that runs `npm ci`, `npm run build`, and copies `.next/standalone`, `.next/static`, and `public` into a non-root runtime image. Expose 3000 and run `node server.js`.

- [ ] **Step 3: Add Compose service and health checks**

Add this health check to `api`:

```yaml
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://localhost:8080/healthz"]
  interval: 5s
  timeout: 3s
  retries: 20
```

Add `frontend` with:

```yaml
frontend:
  build:
    context: ./frontend
  environment:
    API_INTERNAL_URL: http://api:8080
    HOSTNAME: 0.0.0.0
    PORT: "3000"
  ports:
    - "3000:3000"
  depends_on:
    api:
      condition: service_healthy
  healthcheck:
    test: ["CMD", "node", "-e", "fetch('http://localhost:3000').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"]
    interval: 5s
    timeout: 3s
    retries: 20
```

- [ ] **Step 4: Add Make and CI verification**

Add `frontend-test`, `frontend-build`, and `test-all` targets. Extend CI with Node setup using the lockfile cache, `npm ci`, lint, typecheck, unit tests, and production build. Keep the existing Go job unchanged.

- [ ] **Step 5: Verify config and image build**

Run:

```bash
docker compose config --quiet
docker compose build frontend
cd frontend && npm run build
```

Expected: all exit 0.

- [ ] **Step 6: Commit deployment integration**

```bash
git add frontend/Dockerfile frontend/.dockerignore frontend/next.config.ts docker-compose.yml Makefile .github/workflows/ci.yml
git commit -m "build: add frontend to compose and CI"
```

### Task 12: Add Browser Tests and Documentation

**Files:**
- Create: `frontend/playwright.config.ts`
- Create: `frontend/e2e/judge-flow.spec.ts`
- Create: `frontend/e2e/responsive.spec.ts`
- Modify: `README.md`
- Modify: `docs/development-plan.md`

- [ ] **Step 1: Configure Playwright against Compose**

Use `baseURL: process.env.PLAYWRIGHT_BASE_URL ?? "http://localhost:3000"`, Chromium desktop and mobile projects, trace-on-first-retry, and screenshots only on failure.

- [ ] **Step 2: Write end-to-end judge-flow tests**

The test must:

```ts
import { expect, test, type Page } from "@playwright/test";

async function fillMonaco(page: Page, code: string) {
  const input = page.locator(".monaco-editor textarea.inputarea");
  await input.click();
  await page.keyboard.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
  await page.keyboard.type(code);
}

test("submits Python and reaches Accepted", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("link", { name: /A\+B/ }).click();
  await page.getByLabel("Language").selectOption("python");
  await fillMonaco(page, "a, b = map(int, input().split())\nprint(a + b)");
  await page.getByRole("button", { name: "Submit" }).click();
  await expect(page.getByText("Accepted")).toBeVisible({ timeout: 30_000 });
  await page.getByRole("link", { name: "Submissions" }).click();
  await expect(page.getByRole("row", { name: /sum python Accepted/i })).toBeVisible();
});
```

Add Go and C++ cases using their existing accepted sample programs. Use a Monaco helper that clicks the editor and presses the platform select-all shortcut before typing.

- [ ] **Step 3: Write responsive layout tests**

Desktop asserts all three workbench panes are visible and non-overlapping by comparing bounding boxes. Mobile asserts only the selected tabpanel is visible, tab switching works, and no element exceeds `document.documentElement.scrollWidth`.

- [ ] **Step 4: Run the complete stack and browser tests**

Run:

```bash
make judge-images
docker compose up -d --build
cd frontend
npx playwright install chromium
npm run test:e2e
```

Expected: Go, C++, Python, and responsive projects PASS. Redis `XPENDING judge:submissions judge-workers` returns 0 after terminal submissions.

- [ ] **Step 5: Update product documentation**

README must include frontend URL, browser workflow, updated Mermaid architecture, Compose services, test commands, chosen visual direction, and screenshots captured at desktop and mobile widths. Mark Phase 5 implemented in `docs/development-plan.md` without marking authentication, contests, MinIO integration, or Prometheus complete.

- [ ] **Step 6: Run final verification**

Run:

```bash
make test
GOCACHE=$PWD/.cache/go-build go test -race ./...
GOCACHE=$PWD/.cache/go-build go vet ./...
cd frontend && npm run lint && npm run typecheck && npm run test:run && npm run build
docker compose config --quiet
docker compose ps
```

Expected: every command exits 0; API, worker, PostgreSQL, Redis, MinIO, and frontend are running; PostgreSQL and Redis are healthy.

- [ ] **Step 7: Commit the completed frontend demo**

```bash
git add frontend/playwright.config.ts frontend/e2e README.md docs/development-plan.md
git commit -m "docs: complete frontend demo workflow"
```
