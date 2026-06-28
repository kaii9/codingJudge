import type { ComponentType, ReactNode } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, afterEach, beforeAll, beforeEach, expect, it, vi } from "vitest";
import { CodeWorkspace } from "@/components/code-workspace";
import { starterTemplate } from "@/lib/judge";

interface MonacoProps {
  height?: number | string;
  language?: string;
  onChange?: (value?: string) => void;
  options?: Record<string, unknown>;
  path?: string;
  theme?: string;
  value?: string;
}

interface DynamicOptions {
  loading?: () => ReactNode;
  ssr?: boolean;
}

const dynamicHarness = vi.hoisted(() => ({
  calls: [] as Array<{
    loader: () => Promise<unknown>;
    options: DynamicOptions;
  }>,
  monacoProps: [] as MonacoProps[],
  showLoading: false,
}));

vi.mock("next/dynamic", () => ({
  default: (loader: () => Promise<unknown>, options: DynamicOptions) => {
    dynamicHarness.calls.push({ loader, options });

    const DynamicEditor = ((props: MonacoProps) => {
      if (dynamicHarness.showLoading) return options.loading?.() ?? null;

      dynamicHarness.monacoProps.push(props);
      return (
        <textarea
          aria-label={String(props.options?.ariaLabel)}
          data-height={props.height}
          data-language={props.language}
          data-theme={props.theme}
          value={props.value}
          onChange={event => props.onChange?.(event.target.value)}
        />
      );
    }) as ComponentType<MonacoProps>;

    return DynamicEditor;
  },
}));

vi.mock("@monaco-editor/react", () => ({ default: () => null }));

beforeAll(() => {
  const environment = globalThis as typeof globalThis & { jsdom: { window: Window } };
  vi.stubGlobal("localStorage", environment.jsdom.window.localStorage);
});

afterAll(() => vi.unstubAllGlobals());
afterEach(cleanup);

beforeEach(() => {
  localStorage.clear();
  dynamicHarness.monacoProps.length = 0;
  dynamicHarness.showLoading = false;
});

function latestMonacoProps() {
  const props = dynamicHarness.monacoProps.at(-1);
  expect(props).toBeDefined();
  return props!;
}

it("loads Monaco client-only and renders an accessible stable-size fallback", () => {
  expect(dynamicHarness.calls).toHaveLength(1);
  expect(dynamicHarness.calls[0]?.options.ssr).toBe(false);

  dynamicHarness.showLoading = true;
  render(<CodeWorkspace problemId="sum" submitting={false} onSubmit={vi.fn()} />);

  const loading = screen.getByRole("status", { name: "Loading code editor" });
  expect(loading).toHaveStyle({ height: "100%", width: "100%" });
  expect(loading.parentElement).toHaveStyle({ minHeight: "22rem", height: "32rem", width: "100%" });
});

it.each([
  ["go", "Go", "go"],
  ["cpp", "C++", "cpp"],
  ["python", "Python", "python"],
] as const)("uses the %s starter template and Monaco language", async (language, label, monacoLanguage) => {
  const user = userEvent.setup();
  render(<CodeWorkspace problemId="sum" submitting={false} onSubmit={vi.fn()} />);

  const select = screen.getByRole("combobox", { name: "Language" });
  expect(select.tagName).toBe("SELECT");
  await user.selectOptions(select, language);

  expect(screen.getByLabelText("Code editor")).toHaveValue(starterTemplate(language));
  expect(latestMonacoProps()).toMatchObject({
    height: "100%",
    language: monacoLanguage,
    theme: "vs-dark",
  });
  expect(screen.getByRole("option", { name: label })).toHaveValue(language);
});

it("keeps independent language drafts", async () => {
  const user = userEvent.setup();
  render(<CodeWorkspace problemId="sum" submitting={false} onSubmit={vi.fn()} />);
  const editor = screen.getByLabelText("Code editor");

  fireEvent.change(editor, { target: { value: "package main // saved" } });
  await user.selectOptions(screen.getByLabelText("Language"), "python");
  expect(screen.getByLabelText("Code editor")).toHaveValue(starterTemplate("python"));

  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: "print(3)" } });
  await user.selectOptions(screen.getByLabelText("Language"), "cpp");
  expect(screen.getByLabelText("Code editor")).toHaveValue(starterTemplate("cpp"));

  await user.selectOptions(screen.getByLabelText("Language"), "go");
  expect(screen.getByLabelText("Code editor")).toHaveValue("package main // saved");
  await user.selectOptions(screen.getByLabelText("Language"), "python");
  expect(screen.getByLabelText("Code editor")).toHaveValue("print(3)");
});

it("uses stable collision-safe Monaco model paths for each problem and language", async () => {
  const user = userEvent.setup();
  const view = render(
    <CodeWorkspace problemId="arrays/two words" submitting={false} onSubmit={vi.fn()} />,
  );

  const firstGoPath = latestMonacoProps().path;
  expect(firstGoPath).toMatch(/\.go$/);
  expect(firstGoPath).not.toContain("arrays/two words");

  await user.selectOptions(screen.getByLabelText("Language"), "python");
  const firstPythonPath = latestMonacoProps().path;
  expect(firstPythonPath).toMatch(/\.py$/);
  expect(firstPythonPath).not.toBe(firstGoPath);

  await user.selectOptions(screen.getByLabelText("Language"), "go");
  expect(latestMonacoProps().path).toBe(firstGoPath);

  view.rerender(
    <CodeWorkspace problemId="arrays%2Ftwo words" submitting={false} onSubmit={vi.fn()} />,
  );
  expect(latestMonacoProps().path).not.toBe(firstGoPath);

  view.rerender(
    <CodeWorkspace problemId="arrays/two words" submitting={false} onSubmit={vi.fn()} />,
  );
  expect(latestMonacoProps().path).toBe(firstGoPath);
});

it("keeps problem drafts independent when a stale editor callback fires", () => {
  const view = render(
    <CodeWorkspace problemId="sum" submitting={false} onSubmit={vi.fn()} />,
  );

  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: "sum code" } });
  const staleSumOnChange = latestMonacoProps().onChange;

  view.rerender(
    <CodeWorkspace problemId="difference" submitting={false} onSubmit={vi.fn()} />,
  );
  expect(screen.getByLabelText("Code editor")).toHaveValue(starterTemplate("go"));

  act(() => staleSumOnChange?.("late sum code"));
  expect(screen.getByLabelText("Code editor")).toHaveValue(starterTemplate("go"));
  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("late sum code");
  expect(localStorage.getItem("gojudge:draft:difference:go")).toBeNull();

  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: "difference code" } });
  view.rerender(
    <CodeWorkspace problemId="sum" submitting={false} onSubmit={vi.fn()} />,
  );
  expect(screen.getByLabelText("Code editor")).toHaveValue("late sum code");
  expect(localStorage.getItem("gojudge:draft:difference:go")).toBe("difference code");
});

it("submits the active source verbatim and preserves it after completion", async () => {
  const user = userEvent.setup();
  const onSubmit = vi.fn().mockResolvedValue(undefined);
  render(<CodeWorkspace problemId="sum" submitting={false} onSubmit={onSubmit} />);

  await user.selectOptions(screen.getByLabelText("Language"), "python");
  const source = "  print(3)\n";
  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: source } });
  const submit = screen.getByRole("button", { name: "Submit" });

  expect(submit.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  expect(submit).toHaveStyle({ backgroundColor: "#d83a43" });
  await user.click(submit);

  expect(onSubmit).toHaveBeenCalledWith({ language: "python", code: source });
  expect(screen.getByLabelText("Code editor")).toHaveValue(source);
});

it("renders pending state immediately and dispatches rapid clicks only once", async () => {
  let finishSubmission: (() => void) | undefined;
  const onSubmit = vi.fn(() => new Promise<void>(resolve => {
    finishSubmission = resolve;
  }));
  render(<CodeWorkspace problemId="sum" submitting={false} onSubmit={onSubmit} />);
  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: "package main" } });

  const submit = screen.getByRole("button", { name: "Submit" });
  act(() => {
    submit.click();
    submit.click();
  });

  expect(onSubmit).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "Submitting..." })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Submitting..." })).toHaveAttribute("aria-busy", "true");

  await act(async () => finishSubmission?.());
  expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Submit" })).toHaveAttribute("aria-busy", "false");
  expect(screen.getByLabelText("Code editor")).toHaveValue("package main");
});

it("handles a rejected callback and preserves the source", async () => {
  let rejectSubmission: ((reason: Error) => void) | undefined;
  const onSubmit = vi.fn(() => new Promise<void>((_resolve, reject) => {
    rejectSubmission = reject;
  }));
  render(<CodeWorkspace problemId="sum" submitting={false} onSubmit={onSubmit} />);
  const source = "  package main\n";
  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: source } });

  act(() => screen.getByRole("button", { name: "Submit" }).click());
  expect(screen.getByRole("button", { name: "Submitting..." })).toBeDisabled();

  await act(async () => rejectSubmission?.(new Error("request failed")));

  expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Submit" })).toHaveAttribute("aria-busy", "false");
  expect(screen.getByLabelText("Code editor")).toHaveValue(source);
});

it("stays busy after local completion while the parent is submitting", async () => {
  let finishSubmission: (() => void) | undefined;
  const onSubmit = vi.fn(() => new Promise<void>(resolve => {
    finishSubmission = resolve;
  }));
  const view = render(
    <CodeWorkspace problemId="sum" submitting={false} onSubmit={onSubmit} />,
  );
  fireEvent.change(screen.getByLabelText("Code editor"), { target: { value: "package main" } });
  act(() => screen.getByRole("button", { name: "Submit" }).click());

  view.rerender(<CodeWorkspace problemId="sum" submitting onSubmit={onSubmit} />);
  await act(async () => finishSubmission?.());
  expect(screen.getByRole("button", { name: "Submitting..." })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Submitting..." })).toHaveAttribute("aria-busy", "true");

  view.rerender(<CodeWorkspace problemId="sum" submitting={false} onSubmit={onSubmit} />);
  expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled();
});

it("disables submission only for blank source or a parent submission", () => {
  const view = render(
    <CodeWorkspace problemId="sum" submitting={false} onSubmit={vi.fn()} />,
  );
  const editor = screen.getByLabelText("Code editor");

  fireEvent.change(editor, { target: { value: " \n\t " } });
  expect(screen.getByRole("button", { name: "Submit" })).toBeDisabled();

  fireEvent.change(editor, { target: { value: "package main" } });
  expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled();

  view.rerender(
    <CodeWorkspace problemId="sum" submitting onSubmit={vi.fn()} />,
  );
  expect(screen.getByRole("button", { name: "Submitting..." })).toBeDisabled();
});

it("configures Monaco for an accessible stable editing surface", () => {
  render(<CodeWorkspace problemId="sum" submitting={false} onSubmit={vi.fn()} />);

  expect(latestMonacoProps().options).toMatchObject({
    accessibilitySupport: "auto",
    ariaLabel: "Code editor",
    automaticLayout: true,
    fontSize: 14,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
  });
});
