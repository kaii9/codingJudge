import { StrictMode, type ComponentProps } from "react";
import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ProblemPage from "@/app/problems/[id]/page";
import { Workbench } from "@/components/workbench";
import type { CodeWorkspaceProps } from "@/components/code-workspace";
import type { Problem, Submission } from "@/lib/types";

const api = vi.hoisted(() => ({
  getProblems: vi.fn(),
  getProblem: vi.fn(),
  getSubmissions: vi.fn(),
  createSubmission: vi.fn(),
}));

const polling = vi.hoisted(() => ({
  ids: [] as Array<string | null>,
  submission: null as Submission | null,
  error: null as string | null,
  isPolling: false,
  retry: vi.fn(),
}));

vi.mock("@/lib/api", () => api);

vi.mock("@/hooks/use-submission-polling", () => ({
  useSubmissionPolling: (id: string | null) => {
    polling.ids.push(id);
    return {
      submission: polling.submission,
      error: polling.error,
      isPolling: polling.isPolling,
      retry: polling.retry,
    };
  },
}));

vi.mock("@/components/code-workspace", () => ({
  CodeWorkspace: ({ problemId, submitting, onSubmit }: CodeWorkspaceProps) => (
    <section aria-label="Mock code workspace" data-problem-id={problemId}>
      <button
        type="button"
        disabled={submitting}
        onClick={() => void onSubmit({
          language: "python",
          code: "  print('exact')\n\n",
        })}
      >
        {submitting ? "Submitting..." : "Submit"}
      </button>
    </section>
  ),
}));

const sumProblem: Problem = {
  id: "sum",
  title: "A+B",
  description: "Add two numbers",
  language: "go",
  timeLimitMs: 1000,
  memoryLimitMb: 64,
  difficulty: "easy",
  collection: "starter",
  sortOrder: 1,
  tags: ["math"],
};

const echoProblem: Problem = {
  id: "echo",
  title: "Echo",
  description: "Echo the input",
  language: "python",
  timeLimitMs: 1500,
  memoryLimitMb: 128,
  difficulty: "easy",
  collection: "starter",
  sortOrder: 2,
  tags: ["string"],
};

const queuedSubmission: Submission = {
  id: "sub-1",
  problemId: "sum",
  language: "python",
  status: "queued",
  createdAt: "2026-06-27T00:00:00Z",
  updatedAt: "2026-06-27T00:00:00Z",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

function primeSuccessfulLoad(
  problem = sumProblem,
  problems: Problem[] = [sumProblem, echoProblem],
  submissions: Submission[] = [],
) {
  api.getProblems.mockResolvedValue(problems);
  api.getProblem.mockResolvedValue(problem);
  api.getSubmissions.mockResolvedValue(submissions);
}

async function renderReady(problemId = "sum") {
  const view = render(<Workbench problemId={problemId} />);
  await screen.findByRole("heading", { level: 1, name: problemId === "sum" ? "A+B" : "Echo" });
  return view;
}

beforeEach(() => {
  vi.clearAllMocks();
  polling.ids.length = 0;
  polling.submission = null;
  polling.error = null;
  polling.isPolling = false;
  primeSuccessfulLoad();
  api.createSubmission.mockResolvedValue(queuedSubmission);
});

afterEach(cleanup);

describe("Workbench loading", () => {
  it("starts independent initial requests in parallel and renders the selected problem", async () => {
    const problemsRequest = deferred<Problem[]>();
    const problemRequest = deferred<Problem>();
    const submissionsRequest = deferred<Submission[]>();
    api.getProblems.mockReturnValue(problemsRequest.promise);
    api.getProblem.mockReturnValue(problemRequest.promise);
    api.getSubmissions.mockReturnValue(submissionsRequest.promise);

    render(<Workbench problemId="sum" />);

    expect(api.getProblems).toHaveBeenCalledTimes(1);
    expect(api.getProblem).toHaveBeenCalledWith("sum");
    expect(api.getSubmissions).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("main")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("main")).toHaveStyle({ minHeight: "42rem" });
    expect(screen.getByRole("heading", { name: "Loading workbench" })).toBeVisible();

    await act(async () => {
      problemsRequest.resolve([sumProblem, echoProblem]);
      problemRequest.resolve(sumProblem);
      submissionsRequest.resolve([]);
    });

    expect(screen.getByRole("heading", { level: 1, name: "A+B" })).toBeVisible();
    expect(screen.getByText("Add two numbers")).toBeVisible();
    expect(screen.getByRole("link", { name: "A+B" })).toHaveAttribute("aria-current", "page");
  });

  it("renders safe unavailable and unknown states", async () => {
    api.getProblem.mockRejectedValueOnce(new Error("SECRET database detail"));
    const unavailable = render(<Workbench problemId="sum" />);

    expect(await screen.findByRole("heading", { name: "Workbench unavailable" })).toBeVisible();
    expect(screen.getByText("Unable to load the problem workspace. Try again.")).toBeVisible();
    expect(screen.queryByText("SECRET database detail")).not.toBeInTheDocument();

    unavailable.unmount();
    primeSuccessfulLoad();
    api.getProblem.mockRejectedValueOnce({ status: 404, message: "SECRET missing detail" });
    render(<Workbench problemId="missing" />);

    expect(await screen.findByRole("heading", { name: "Problem not found" })).toBeVisible();
    expect(screen.getByText("This problem is unavailable or does not exist.")).toBeVisible();
    expect(screen.queryByText("SECRET missing detail")).not.toBeInTheDocument();
  });

  it("keeps the selected problem available when auxiliary initial loads fail", async () => {
    const problemsRequest = deferred<Problem[]>();
    const submissionsRequest = deferred<Submission[]>();
    api.getProblems.mockReturnValueOnce(problemsRequest.promise);
    api.getSubmissions.mockReturnValueOnce(submissionsRequest.promise);

    render(<Workbench problemId="sum" />);

    expect(await screen.findByRole("heading", { level: 1, name: "A+B" })).toBeVisible();
    await act(async () => {
      problemsRequest.reject({
        status: 404,
        message: "SECRET problem-list detail",
      });
      submissionsRequest.reject(new Error("SECRET submission-history detail"));
    });
    expect(screen.queryByRole("heading", { name: "Problem not found" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Workbench unavailable" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "A+B" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("region", { name: "Recent submissions" })).toHaveTextContent(
      "No recent submissions.",
    );
    expect(await screen.findByRole("status", { name: "Workspace warning" })).toHaveTextContent(
      "Some workspace data is unavailable.",
    );
    expect(screen.queryByText("SECRET problem-list detail")).not.toBeInTheDocument();
    expect(screen.queryByText("SECRET submission-history detail")).not.toBeInTheDocument();
  });

  it("retries a failed initial load", async () => {
    const user = userEvent.setup();
    api.getProblem.mockRejectedValueOnce(new Error("offline"));
    render(<Workbench problemId="sum" />);

    await user.click(await screen.findByRole("button", { name: "Retry loading" }));

    expect(await screen.findByRole("heading", { level: 1, name: "A+B" })).toBeVisible();
    expect(api.getProblem).toHaveBeenCalledTimes(2);
  });

  it("starts a fresh problem request when an auxiliary initial load never settles", async () => {
    const user = userEvent.setup();
    api.getProblems.mockReturnValue(new Promise<Problem[]>(() => {}));
    api.getProblem
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(sumProblem);

    render(<Workbench problemId="retry-race" />);

    await user.click(await screen.findByRole("button", { name: "Retry loading" }));

    await waitFor(() => expect(api.getProblem).toHaveBeenCalledTimes(2));
    expect(api.getProblem).toHaveBeenLastCalledWith("retry-race");
  });

  it("ignores a stale load after the problem changes", async () => {
    const oldProblems = deferred<Problem[]>();
    const oldProblem = deferred<Problem>();
    const oldSubmissions = deferred<Submission[]>();
    api.getProblems.mockReturnValueOnce(oldProblems.promise).mockResolvedValueOnce([echoProblem]);
    api.getProblem.mockReturnValueOnce(oldProblem.promise).mockResolvedValueOnce(echoProblem);
    api.getSubmissions.mockReturnValueOnce(oldSubmissions.promise).mockResolvedValueOnce([]);

    const view = render(<Workbench problemId="sum" />);
    view.rerender(<Workbench problemId="echo" />);

    expect(await screen.findByRole("heading", { level: 1, name: "Echo" })).toBeVisible();

    await act(async () => {
      oldProblems.resolve([sumProblem]);
      oldProblem.resolve(sumProblem);
      oldSubmissions.resolve([]);
    });

    expect(screen.getByRole("heading", { level: 1, name: "Echo" })).toBeVisible();
    expect(screen.queryByRole("heading", { level: 1, name: "A+B" })).not.toBeInTheDocument();
  });

  it("settles an obsolete load safely after unmount", async () => {
    const problemRequest = deferred<Problem>();
    api.getProblem.mockReturnValue(problemRequest.promise);
    const view = render(<Workbench problemId="sum" />);

    view.unmount();
    await act(async () => problemRequest.resolve(sumProblem));

    expect(api.getProblem).toHaveBeenCalledTimes(1);
  });
});

describe("Workbench submissions", () => {
  it("submits the exact source and starts polling only after create succeeds", async () => {
    const user = userEvent.setup();
    const createRequest = deferred<Submission>();
    api.createSubmission.mockReturnValue(createRequest.promise);
    await renderReady();
    polling.ids.length = 0;

    await user.click(screen.getByRole("button", { name: "Submit" }));

    expect(api.createSubmission).toHaveBeenCalledWith({
      problemId: "sum",
      language: "python",
      code: "  print('exact')\n\n",
    });
    expect(polling.ids).not.toContain("sub-1");

    await act(async () => createRequest.resolve(queuedSubmission));

    expect(polling.ids).toContain("sub-1");
    expect(screen.getByText("Queued")).toBeVisible();
  });

  it("keeps create failures separate and allows a new submission", async () => {
    const user = userEvent.setup();
    api.createSubmission
      .mockRejectedValueOnce(new Error("SECRET create detail"))
      .mockResolvedValueOnce(queuedSubmission);
    await renderReady();

    await user.click(screen.getByRole("button", { name: "Submit" }));

    const submitError = await screen.findByRole("alert", { name: "Submission error" });
    expect(submitError).toHaveTextContent("Submission could not be created. Try again.");
    expect(
      within(screen.getByRole("tabpanel", { name: "Code" }))
        .getByRole("alert", { name: "Submission error" }),
    ).toBe(submitError);
    expect(screen.queryByText("SECRET create detail")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Submit" })).toBeEnabled();

    await user.click(screen.getByRole("button", { name: "Submit" }));

    await waitFor(() => expect(polling.ids).toContain("sub-1"));
    expect(api.createSubmission).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("alert", { name: "Submission error" })).not.toBeInTheDocument();
  });

  it("returns to Code before showing a deferred create failure", async () => {
    const user = userEvent.setup();
    const createRequest = deferred<Submission>();
    api.createSubmission.mockReturnValue(createRequest.promise);
    await renderReady();

    const codeTab = screen.getByRole("tab", { name: "Code" });
    const resultTab = screen.getByRole("tab", { name: "Result" });
    await user.click(codeTab);
    await user.click(screen.getByRole("button", { name: "Submit" }));
    await user.click(resultTab);
    expect(resultTab).toHaveAttribute("aria-selected", "true");

    await act(async () => createRequest.reject(new Error("SECRET delayed rejection")));

    const submitError = await screen.findByRole("alert", { name: "Submission error" });
    expect(codeTab).toHaveAttribute("aria-selected", "true");
    expect(resultTab).toHaveAttribute("aria-selected", "false");
    expect(within(screen.getByRole("tabpanel", { name: "Code" })).getByRole("alert"))
      .toBe(submitError);
    expect(api.createSubmission).toHaveBeenCalledWith({
      problemId: "sum",
      language: "python",
      code: "  print('exact')\n\n",
    });
    expect(screen.queryByText("SECRET delayed rejection")).not.toBeInTheDocument();
  });

  it("does not let a stale create overwrite a new problem", async () => {
    const user = userEvent.setup();
    const createRequest = deferred<Submission>();
    api.createSubmission.mockReturnValue(createRequest.promise);
    const view = await renderReady();

    await user.click(screen.getByRole("button", { name: "Submit" }));
    primeSuccessfulLoad(echoProblem, [echoProblem]);
    view.rerender(<Workbench problemId="echo" />);
    await screen.findByRole("heading", { level: 1, name: "Echo" });
    polling.ids.length = 0;

    await act(async () => createRequest.resolve(queuedSubmission));

    expect(polling.ids).not.toContain("sub-1");
    expect(screen.getByRole("heading", { level: 1, name: "Echo" })).toBeVisible();
    expect(screen.queryByText("Queued")).not.toBeInTheDocument();
  });

  it("does not apply a create result after unmount", async () => {
    const user = userEvent.setup();
    const createRequest = deferred<Submission>();
    api.createSubmission.mockReturnValue(createRequest.promise);
    const view = await renderReady();

    await user.click(screen.getByRole("button", { name: "Submit" }));
    view.unmount();
    polling.ids.length = 0;
    await act(async () => createRequest.resolve(queuedSubmission));

    expect(polling.ids).not.toContain("sub-1");
  });

  it("does not refresh history for a nonterminal active submission", async () => {
    const user = userEvent.setup();
    const view = await renderReady();
    api.getSubmissions.mockClear();

    await user.click(screen.getByRole("button", { name: "Submit" }));
    polling.submission = { ...queuedSubmission, status: "running" };
    view.rerender(<Workbench problemId="sum" />);

    expect(api.getSubmissions).not.toHaveBeenCalled();
  });

  it("refreshes history once for a terminal active submission across duplicate renders", async () => {
    const user = userEvent.setup();
    const view = render(
      <StrictMode>
        <Workbench problemId="sum" />
      </StrictMode>,
    );
    await screen.findByRole("heading", { level: 1, name: "A+B" });
    api.getSubmissions.mockClear();

    await user.click(screen.getByRole("button", { name: "Submit" }));
    polling.submission = {
      ...queuedSubmission,
      status: "accepted",
      result: { status: "accepted", stdout: "3\n" },
    };
    view.rerender(
      <StrictMode>
        <Workbench problemId="sum" />
      </StrictMode>,
    );
    view.rerender(
      <StrictMode>
        <Workbench problemId="sum" />
      </StrictMode>,
    );

    await waitFor(() => expect(api.getSubmissions).toHaveBeenCalledTimes(1));
  });

  it("ignores a stale terminal refresh after switching problems", async () => {
    const user = userEvent.setup();
    const view = await renderReady();
    await user.click(screen.getByRole("button", { name: "Submit" }));

    const oldRefresh = deferred<Submission[]>();
    api.getSubmissions.mockReturnValueOnce(oldRefresh.promise);
    polling.submission = {
      ...queuedSubmission,
      status: "accepted",
      result: { status: "accepted" },
    };
    view.rerender(<Workbench problemId="sum" />);
    await waitFor(() => expect(api.getSubmissions).toHaveBeenCalledTimes(2));

    primeSuccessfulLoad(echoProblem, [echoProblem], []);
    polling.submission = null;
    view.rerender(<Workbench problemId="echo" />);
    await screen.findByRole("heading", { level: 1, name: "Echo" });

    await act(async () => oldRefresh.resolve([
      { ...queuedSubmission, id: "stale-submission", status: "wrong_answer" },
    ]));

    const recent = screen.getByRole("region", { name: "Recent submissions" });
    expect(within(recent).getByText("No recent submissions.")).toBeVisible();
    expect(within(recent).queryByText("Wrong Answer")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Echo" })).toBeVisible();
  });

  it("prevents an older same-problem refresh from overwriting newer history", async () => {
    const user = userEvent.setup();
    const view = await renderReady();
    const firstRefresh = deferred<Submission[]>();
    const secondRefresh = deferred<Submission[]>();
    const firstSubmission = { ...queuedSubmission, id: "sub-a" };
    const secondSubmission = { ...queuedSubmission, id: "sub-b" };
    api.createSubmission.mockReset();
    api.createSubmission
      .mockResolvedValueOnce(firstSubmission)
      .mockResolvedValueOnce(secondSubmission);
    api.getSubmissions.mockReset();
    api.getSubmissions
      .mockReturnValueOnce(firstRefresh.promise)
      .mockReturnValueOnce(secondRefresh.promise);

    await user.click(screen.getByRole("button", { name: "Submit" }));
    await waitFor(() => expect(polling.ids).toContain("sub-a"));
    polling.submission = {
      ...firstSubmission,
      status: "accepted",
      result: { status: "accepted" },
    };
    view.rerender(<Workbench problemId="sum" />);
    await waitFor(() => expect(api.getSubmissions).toHaveBeenCalledTimes(1));

    await user.click(screen.getByRole("button", { name: "Submit" }));
    await waitFor(() => expect(polling.ids).toContain("sub-b"));
    polling.submission = {
      ...secondSubmission,
      status: "accepted",
      result: { status: "accepted" },
    };
    view.rerender(<Workbench problemId="sum" />);
    await waitFor(() => expect(api.getSubmissions).toHaveBeenCalledTimes(2));

    await act(async () => secondRefresh.resolve([
      { ...secondSubmission, id: "history-entry", status: "accepted" },
    ]));
    const recent = screen.getByRole("region", { name: "Recent submissions" });
    expect(within(recent).getByText("Accepted")).toBeVisible();

    await act(async () => firstRefresh.resolve([
      { ...firstSubmission, id: "history-entry", status: "wrong_answer" },
    ]));

    expect(within(recent).getByText("Accepted")).toBeVisible();
    expect(within(recent).queryByText("Wrong Answer")).not.toBeInTheDocument();
  });

  it("prevents late initial history from overwriting a terminal refresh", async () => {
    const user = userEvent.setup();
    const initialHistory = deferred<Submission[]>();
    const refreshedSubmission = {
      ...queuedSubmission,
      id: "history-entry",
      status: "accepted" as const,
    };
    api.getSubmissions
      .mockReturnValueOnce(initialHistory.promise)
      .mockResolvedValueOnce([refreshedSubmission]);
    const view = await renderReady();

    await user.click(screen.getByRole("button", { name: "Submit" }));
    polling.submission = {
      ...queuedSubmission,
      status: "accepted",
      result: { status: "accepted" },
    };
    view.rerender(<Workbench problemId="sum" />);

    await waitFor(() => expect(api.getSubmissions).toHaveBeenCalledTimes(2));
    const recent = screen.getByRole("region", { name: "Recent submissions" });
    expect(await within(recent).findByText("Accepted")).toBeVisible();

    await act(async () => initialHistory.resolve([
      { ...refreshedSubmission, status: "wrong_answer" },
    ]));

    expect(within(recent).getByText("Accepted")).toBeVisible();
    expect(within(recent).queryByText("Wrong Answer")).not.toBeInTheDocument();
  });

  it("passes polling failures through the retained result panel", async () => {
    const user = userEvent.setup();
    const view = await renderReady();
    await user.click(screen.getByRole("button", { name: "Submit" }));
    polling.submission = { ...queuedSubmission, status: "running" };
    polling.error = "SECRET polling detail";

    view.rerender(<Workbench problemId="sum" />);

    expect(screen.getByText("Running")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(polling.retry).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("SECRET polling detail")).not.toBeInTheDocument();
  });
});

describe("Workbench layout", () => {
  it("bounds recent submissions", async () => {
    const recent = Array.from({ length: 12 }, (_, index): Submission => ({
      ...queuedSubmission,
      id: `recent-${index}`,
    }));
    primeSuccessfulLoad(sumProblem, [sumProblem], recent);

    await renderReady();

    const region = screen.getByRole("region", { name: "Recent submissions" });
    expect(within(region).getAllByRole("listitem")).toHaveLength(8);
  });

  it("provides accessible Problem, Code, and Result mobile tab selection", async () => {
    const user = userEvent.setup();
    await renderReady();

    const tabs = screen.getByRole("tablist", { name: "Workbench views" });
    const problemTab = within(tabs).getByRole("tab", { name: "Problem" });
    const codeTab = within(tabs).getByRole("tab", { name: "Code" });
    const resultTab = within(tabs).getByRole("tab", { name: "Result" });
    expect(problemTab).toHaveAttribute("aria-selected", "true");
    expect(codeTab).toHaveAttribute("aria-selected", "false");
    expect(resultTab).toHaveAttribute("aria-selected", "false");

    await user.click(codeTab);

    expect(problemTab).toHaveAttribute("aria-selected", "false");
    expect(codeTab).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel", { name: "Code" })).toHaveAttribute("data-active", "true");

    await user.click(resultTab);
    expect(resultTab).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel", { name: "Result" })).toHaveAttribute("data-active", "true");
  });

  it("supports roving keyboard navigation and focuses the selected tab", async () => {
    const user = userEvent.setup();
    await renderReady();

    const problemTab = screen.getByRole("tab", { name: "Problem" });
    const codeTab = screen.getByRole("tab", { name: "Code" });
    const resultTab = screen.getByRole("tab", { name: "Result" });
    problemTab.focus();

    await user.keyboard("{ArrowRight}");
    expect(codeTab).toHaveFocus();
    expect(codeTab).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{End}");
    expect(resultTab).toHaveFocus();
    expect(resultTab).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{Home}");
    expect(problemTab).toHaveFocus();
    expect(problemTab).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{ArrowLeft}");
    expect(resultTab).toHaveFocus();
    expect(resultTab).toHaveAttribute("aria-selected", "true");

    await user.keyboard("{ArrowRight}");
    expect(problemTab).toHaveFocus();
    expect(problemTab).toHaveAttribute("aria-selected", "true");
    expect(problemTab).toHaveAttribute("aria-controls", "workbench-problem-panel");
  });

  it("shares only in-flight initial GETs during StrictMode replay", async () => {
    const firstMount = render(
      <StrictMode>
        <Workbench problemId="sum" />
      </StrictMode>,
    );

    expect(await screen.findByRole("heading", { level: 1, name: "A+B" })).toBeVisible();
    expect(api.getProblems).toHaveBeenCalledTimes(1);
    expect(api.getProblem).toHaveBeenCalledTimes(1);
    expect(api.getSubmissions).toHaveBeenCalledTimes(1);

    firstMount.unmount();
    render(<Workbench problemId="sum" />);
    expect(await screen.findByRole("heading", { level: 1, name: "A+B" })).toBeVisible();
    expect(api.getProblems).toHaveBeenCalledTimes(2);
    expect(api.getProblem).toHaveBeenCalledTimes(2);
    expect(api.getSubmissions).toHaveBeenCalledTimes(2);
  });

  it("uses stable left, center, right, and bottom workbench regions", async () => {
    await renderReady();

    const workbench = screen.getByRole("main");
    expect(workbench.querySelector(".workbench__rail")).not.toBeNull();
    expect(workbench.querySelector(".workbench__problem")).not.toBeNull();
    expect(workbench.querySelector(".workbench__code")).not.toBeNull();
    expect(workbench.querySelector(".workbench__result")).not.toBeNull();
  });
});

describe("problem route", () => {
  it("awaits Next 16 params and passes the id directly to Workbench", async () => {
    const params = deferred<{ id: string }>();
    const pagePromise = ProblemPage({ params: params.promise } as ComponentProps<typeof ProblemPage>);
    params.resolve({ id: "arrays/two words" });

    const page = await pagePromise;

    expect(page.type).toBe(Workbench);
    expect(page.props).toEqual({ problemId: "arrays/two words" });
  });
});
