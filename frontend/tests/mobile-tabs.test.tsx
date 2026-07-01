import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Workbench } from "@/components/workbench";
import { WorkbenchTabs, type WorkbenchTab } from "@/components/workbench-tabs";
import type { CreateSubmissionInput, Problem, Submission } from "@/lib/types";

const api = vi.hoisted(() => ({
  createSubmission: vi.fn(),
  getProblem: vi.fn(),
  getProblems: vi.fn(),
  getSubmissions: vi.fn(),
}));

vi.mock("@/lib/api", () => api);
vi.mock("@/hooks/use-submission-polling", () => ({
  useSubmissionPolling: () => ({
    submission: null,
    error: null,
    isPolling: false,
    retry: vi.fn(),
  }),
}));
vi.mock("@/components/code-workspace", () => ({
  CodeWorkspace: ({
    submitting,
    onSubmit,
  }: {
    submitting: boolean;
    onSubmit: (input: Pick<CreateSubmissionInput, "language" | "code">) => Promise<void>;
  }) => (
    <button
      type="button"
      disabled={submitting}
      onClick={() => void onSubmit({ language: "go", code: "package main" })}
    >
      {submitting ? "Submitting..." : "Submit"}
    </button>
  ),
}));

const globalsCss = readFileSync(resolve("app/globals.css"), "utf8");
const workbenchSource = readFileSync(resolve("components/workbench.tsx"), "utf8");

const tabs: ReadonlyArray<{
  id: WorkbenchTab;
  label: string;
  panelId: string;
}> = [
  { id: "problem", label: "Problem", panelId: "workbench-problem-panel" },
  { id: "code", label: "Code", panelId: "workbench-code-panel" },
  { id: "result", label: "Result", panelId: "workbench-result-panel" },
];

const problem: Problem = {
  id: "sum",
  title: "A+B",
  description: "Add two numbers",
  language: "go",
  timeLimitMs: 1000,
  memoryLimitMb: 64,
};

const queuedSubmission: Submission = {
  id: "sub-focus",
  problemId: "sum",
  language: "go",
  status: "queued",
  createdAt: "2026-06-30T00:00:00Z",
  updatedAt: "2026-06-30T00:00:00Z",
};

function deferred<T>() {
  let resolvePromise!: (value: T) => void;
  const promise = new Promise<T>(resolve => {
    resolvePromise = resolve;
  });
  return { promise, resolve: resolvePromise };
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getProblems.mockResolvedValue([problem]);
  api.getProblem.mockResolvedValue(problem);
  api.getSubmissions.mockResolvedValue([]);
});

afterEach(cleanup);

describe("WorkbenchTabs", () => {
  it("reports clicks and reflects a controlled rerender", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const view = render(<WorkbenchTabs active="problem" onChange={onChange} />);

    await user.click(screen.getByRole("tab", { name: "Code" }));

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenLastCalledWith("code");
    expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    view.rerender(<WorkbenchTabs active="code" onChange={onChange} />);

    expect(screen.getByRole("tab", { name: "Code" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tab", { name: "Problem" })).toHaveAttribute(
      "aria-selected",
      "false",
    );
  });

  it("exposes stable tab and panel relationships with roving tab stops", () => {
    render(<WorkbenchTabs active="result" onChange={vi.fn()} />);

    const tablist = screen.getByRole("tablist", { name: "Workbench views" });
    for (const tab of tabs) {
      const control = within(tablist).getByRole("tab", { name: tab.label });
      expect(control).toHaveAttribute("id", `workbench-${tab.id}-tab`);
      expect(control).toHaveAttribute("aria-controls", tab.panelId);
      expect(control).toHaveAttribute(
        "aria-selected",
        tab.id === "result" ? "true" : "false",
      );
      expect(control).toHaveAttribute("tabindex", tab.id === "result" ? "0" : "-1");
    }
  });

  it.each([
    ["problem", "{ArrowRight}", "code"],
    ["problem", "{ArrowLeft}", "result"],
    ["result", "{ArrowRight}", "problem"],
    ["problem", "{Home}", "problem"],
    ["problem", "{End}", "result"],
  ] as const)(
    "moves from %s with %s to %s and calls onChange once",
    async (active, key, expected) => {
      const user = userEvent.setup();
      const onChange = vi.fn();
      const view = render(<WorkbenchTabs active={active} onChange={onChange} />);
      screen.getByRole("tab", { name: tabs.find(tab => tab.id === active)?.label }).focus();

      await user.keyboard(key);

      expect(onChange).toHaveBeenCalledTimes(1);
      expect(onChange).toHaveBeenCalledWith(expected);
      expect(screen.getByRole("tab", {
        name: tabs.find(tab => tab.id === expected)?.label,
      })).toHaveFocus();

      view.rerender(<WorkbenchTabs active={expected} onChange={onChange} />);
      expect(screen.getByRole("tab", {
        name: tabs.find(tab => tab.id === expected)?.label,
      })).toHaveAttribute("aria-selected", "true");
    },
  );

  it("prevents the browser default for handled navigation keys", () => {
    const onChange = vi.fn();
    render(<WorkbenchTabs active="code" onChange={onChange} />);

    const handled = fireEvent.keyDown(screen.getByRole("tab", { name: "Code" }), {
      key: "ArrowRight",
    });

    expect(handled).toBe(false);
    expect(onChange).toHaveBeenCalledTimes(1);
  });
});

describe("Workbench tab integration", () => {
  it("reuses WorkbenchTabs instead of duplicating tab controls", () => {
    expect(workbenchSource).toContain(
      'import { WorkbenchTabs, type WorkbenchTab } from "@/components/workbench-tabs";',
    );
    expect(workbenchSource).toMatch(
      /<WorkbenchTabs\s+active=\{mobileTab\}\s+onChange=\{handleTabChange\}\s*\/>/,
    );
    expect(workbenchSource).not.toContain("workbenchTabs.map");
    expect(workbenchSource).not.toContain("keyboardTabTarget");
  });

  it.each(tabs)("matches the $label tab to its panel contract", tab => {
    const panel = new RegExp(
      `id="${tab.panelId}"[\\s\\S]*?role="tabpanel"[\\s\\S]*?`
      + `aria-labelledby="workbench-${tab.id}-tab"[\\s\\S]*?`
      + `data-active=\\{mobileTab === "${tab.id}"\\}`,
    );

    expect(workbenchSource).toMatch(panel);
  });

  it("moves focus out of Code after a successful submission activates Result", async () => {
    const user = userEvent.setup();
    const createRequest = deferred<Submission>();
    api.createSubmission.mockReturnValue(createRequest.promise);
    render(<Workbench problemId="sum" />);
    await screen.findByRole("heading", { level: 1, name: "A+B" });

    await user.click(screen.getByRole("tab", { name: "Code" }));
    const submit = screen.getByRole("button", { name: "Submit" });
    await user.click(submit);
    expect(submit).toHaveFocus();

    await act(async () => createRequest.resolve(queuedSubmission));

    const resultPanel = screen.getByRole("tabpanel", { name: "Result" });
    expect(resultPanel).toHaveAttribute("data-active", "true");
    await waitFor(() => expect(resultPanel).toHaveFocus());
    expect(resultPanel).toHaveAttribute("tabindex", "-1");
  });
});

describe("responsive workbench CSS contract", () => {
  const compactCss = globalsCss.replace(/\s+/g, " ");

  it("declares explicit desktop tracks and a stable result row", () => {
    expect(compactCss).toMatch(
      /\.workbench \{[^}]*grid-template-columns: minmax\(180px, 240px\) minmax\(320px, 0\.9fr\) minmax\(420px, 1\.1fr\)/,
    );
    expect(compactCss).toMatch(
      /\.workbench \{[^}]*grid-template-rows: minmax\(0, 1fr\) minmax\(16rem, 16rem\)/,
    );
    expect(compactCss).toMatch(/\.workbench \{[^}]*min-height:[^;]+;[^}]*max-height:/);
  });

  it("declares independent constrained scrolling for every desktop pane", () => {
    expect(compactCss).toMatch(
      /\.workbench__rail, \.workbench__problem, \.workbench__code, \.workbench__result \{[^}]*min-width: 0;[^}]*min-height: 0;[^}]*overflow: auto;/,
    );
    expect(compactCss).toMatch(
      /\.workbench__code \.code-workspace__editor \{[^}]*height: 100%(?: !important)?;[^}]*min-height: 20rem(?: !important)?;/,
    );
    expect(compactCss).toMatch(
      /\.workbench__result \.judge-result-panel \{[^}]*height: 100%;[^}]*min-height: 0(?: !important)?;/,
    );
  });

  it("places the single-pane CSS breakpoint safely above the desktop track minimum", () => {
    const desktopColumns = globalsCss.match(
      /grid-template-columns:\s*minmax\((\d+)px,\s*\d+px\)\s+minmax\((\d+)px,[^)]+\)\s+minmax\((\d+)px,[^)]+\)/,
    );
    const singlePaneMedia = globalsCss.match(
      /@media \(max-width:\s*(\d+)px\)\s*\{\s*\.workbench\s*\{[^}]*grid-template-columns:\s*minmax\(0,\s*1fr\)/,
    );

    expect(desktopColumns).not.toBeNull();
    expect(singlePaneMedia).not.toBeNull();
    const desktopMinimum = desktopColumns!.slice(1).reduce(
      (total, width) => total + Number(width),
      0,
    );
    expect(Number(singlePaneMedia![1])).toBeGreaterThan(desktopMinimum);
    expect(compactCss).not.toMatch(/\.workbench \{[^}]*overflow(?:-x)?: (?:hidden|auto|scroll);/);
    expect(globalsCss).not.toMatch(/overflow-x:\s*(?:hidden|auto|scroll)/);
  });

  it("declares one active pane and 44px controls throughout the sub-900px range", () => {
    expect(compactCss).toContain("@media (max-width: 960px)");
    expect(compactCss).toMatch(
      /@media \(max-width: 960px\)[\s\S]*?\.workbench \{[^}]*grid-template-columns: minmax\(0, 1fr\)/,
    );
    expect(compactCss).toMatch(
      /@media \(max-width: 960px\)[\s\S]*?\.workbench__tabs \{[^}]*display: flex;/,
    );
    expect(compactCss).toMatch(
      /\.workbench \[role="tabpanel"\]\[data-active="false"\] \{ display: none; \}/,
    );
    expect(compactCss).toMatch(
      /@media \(max-width: 960px\)[\s\S]*?\.workbench__tabs button \{[^}]*min-height: 44px;/,
    );
  });

  it("puts a 100vh fallback immediately before every 100dvh sizing declaration", () => {
    expect(compactCss).toMatch(
      /\.app-shell \{[^}]*min-height: 100vh; min-height: 100dvh;/,
    );

    const dynamicViewportDeclarations = [...globalsCss.matchAll(
      /^\s*(min-height|height):\s*([^;]*100dvh[^;]*);/gm,
    )];
    expect(dynamicViewportDeclarations.length).toBeGreaterThanOrEqual(5);

    for (const declaration of dynamicViewportDeclarations) {
      const precedingCss = globalsCss.slice(0, declaration.index);
      const fallback = precedingCss.match(/(min-height|height):\s*([^;]+);\s*$/);
      expect(fallback?.[1]).toBe(declaration[1]);
      expect(fallback?.[2].trim()).toBe(declaration[2].replaceAll("100dvh", "100vh").trim());
    }
  });

  it("preserves focus, reduced-motion, and typography accessibility contracts", () => {
    expect(compactCss).toMatch(
      /:focus-visible \{[^}]*outline: (?:2px|3px) solid var\(--color-focus-ring\)/,
    );
    expect(compactCss).toContain("@media (prefers-reduced-motion: reduce)");

    const letterSpacing = [...globalsCss.matchAll(/letter-spacing:\s*([^;]+);/g)];
    expect(letterSpacing.length).toBeGreaterThan(0);
    expect(letterSpacing.every(([, value]) => value.trim() === "0")).toBe(true);
    expect(globalsCss).not.toMatch(/font-size\s*:[^;]*(?:\dvw|clamp\([^;]*vw)/i);
  });
});
