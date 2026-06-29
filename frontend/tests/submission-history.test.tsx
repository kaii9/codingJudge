import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode, type ComponentType } from "react";
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import {
  SubmissionHistory,
  type SubmissionHistoryProps,
} from "@/components/submission-history";
import type { JudgeResult, Submission } from "@/lib/types";

const api = vi.hoisted(() => ({
  getSubmissions: vi.fn(),
}));

vi.mock("@/lib/api", () => api);

const newestSubmission: Submission & {
  sourceCode: string;
  hiddenTestData: string;
  expectedOutput: string;
  result: JudgeResult & { hiddenInput: string; expectedOutput: string };
} = {
  id: "sub/newest with space",
  problemId: "arrays/two words",
  language: "go",
  status: "accepted",
  result: {
    status: "accepted",
    durationMs: 0,
    stdout: "PRIVATE STDOUT SENTINEL",
    stderr: "PRIVATE STDERR SENTINEL",
    hiddenInput: "PRIVATE INPUT SENTINEL",
    expectedOutput: "PRIVATE RESULT EXPECTED SENTINEL",
  },
  createdAt: "2026-06-29T10:20:30Z",
  updatedAt: "2026-06-29T10:20:31Z",
  sourceCode: "PRIVATE SOURCE SENTINEL",
  hiddenTestData: "PRIVATE TEST SENTINEL",
  expectedOutput: "PRIVATE EXPECTED SENTINEL",
};

const olderSubmission: Submission = {
  id: "sub-%2F-older",
  problemId: "arrays%2Ftwo words",
  language: "cpp",
  status: "wrong_answer",
  result: { status: "wrong_answer" },
  createdAt: "2026-06-28T09:10:11Z",
  updatedAt: "2026-06-28T09:10:12Z",
};

const runningSubmission: Submission = {
  id: "sub-running",
  problemId: "echo",
  language: "python",
  status: "running",
  createdAt: "2026-06-27T08:09:10Z",
  updatedAt: "2026-06-27T08:09:10Z",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, reject, resolve };
}

afterEach(cleanup);

describe("SubmissionHistory", () => {
  it("renders the supplied newest-first order without mutating the input", () => {
    const submissions = Object.freeze([
      Object.freeze({ ...newestSubmission }),
      Object.freeze({ ...olderSubmission }),
      Object.freeze({ ...runningSubmission }),
    ]) satisfies readonly Submission[];
    const originalIds = submissions.map(({ id }) => id);

    const { container } = render(
      <SubmissionHistory
        submissions={submissions}
        loading={false}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    const table = screen.getByRole("table", { name: "Submission history" });
    expect(within(table).getAllByRole("columnheader").map(cell => cell.textContent)).toEqual([
      "Submission",
      "Problem",
      "Language",
      "Status",
      "Submitted",
      "Duration",
    ]);
    expect(
      within(table).getAllByRole("row").slice(1).map(row =>
        within(row).getByTestId("submission-id").textContent),
    ).toEqual(originalIds);
    expect(submissions.map(({ id }) => id)).toEqual(originalIds);
    expect(container.querySelectorAll("table")).toHaveLength(1);
  });

  it("uses collision-safe encoded problem links", () => {
    render(
      <SubmissionHistory
        submissions={[newestSubmission, olderSubmission]}
        loading={false}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByRole("link", { name: "arrays/two words" })).toHaveAttribute(
      "href",
      "/problems/arrays%2Ftwo%20words",
    );
    expect(screen.getByRole("link", { name: "arrays%2Ftwo words" })).toHaveAttribute(
      "href",
      "/problems/arrays%252Ftwo%20words",
    );
  });

  it("shows canonical languages, statuses, timestamps, and available duration including zero", () => {
    const { container } = render(
      <SubmissionHistory
        submissions={[newestSubmission, olderSubmission, runningSubmission]}
        loading={false}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByText("Go")).toBeVisible();
    expect(screen.getByText("C++")).toBeVisible();
    expect(screen.getByText("Python")).toBeVisible();
    expect(screen.getByText("Accepted")).toBeVisible();
    expect(screen.getByText("Wrong Answer")).toBeVisible();
    expect(screen.getByText("Running")).toBeVisible();
    expect(screen.getByText("0 ms")).toBeVisible();

    const timestamps = Array.from(container.querySelectorAll("time"));
    expect(timestamps.map(timestamp => timestamp.getAttribute("datetime"))).toEqual([
      newestSubmission.createdAt,
      olderSubmission.createdAt,
      runningSubmission.createdAt,
    ]);
    expect(timestamps.every(timestamp => (timestamp.textContent?.length ?? 0) > 0)).toBe(true);

    const olderRow = screen.getByText(olderSubmission.id).closest("tr");
    expect(olderRow).not.toBeNull();
    expect(within(olderRow!).queryByText(/\d+ ms/)).not.toBeInTheDocument();
  });

  it("does not expose source, process output, or test data", () => {
    render(
      <SubmissionHistory
        submissions={[newestSubmission]}
        loading={false}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    for (const privateValue of [
      newestSubmission.sourceCode,
      newestSubmission.result.stdout,
      newestSubmission.result.stderr,
      newestSubmission.result.hiddenInput,
      newestSubmission.hiddenTestData,
      newestSubmission.expectedOutput,
      newestSubmission.result.expectedOutput,
    ]) {
      expect(screen.queryByText(privateValue!)).not.toBeInTheDocument();
    }
  });

  it("renders one quiet, stable loading skeleton", () => {
    const { container } = render(
      <SubmissionHistory
        submissions={[]}
        loading
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(screen.getByRole("status", { name: "Loading submissions" })).toHaveAttribute(
      "aria-live",
      "polite",
    );
    const skeleton = container.querySelector(".submission-history__skeleton");
    expect(skeleton).toHaveAttribute("aria-hidden", "true");
    expect(skeleton?.querySelectorAll(".submission-history__skeleton-row")).toHaveLength(4);
  });

  it("renders the empty state when loading has completed", () => {
    render(
      <SubmissionHistory
        submissions={[]}
        loading={false}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByText("No submissions yet")).toBeVisible();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("shows a generic retryable error without leaking details", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    render(
      <SubmissionHistory
        submissions={[]}
        loading={false}
        error="SECRET database connection detail"
        onRetry={onRetry}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Unable to load submissions. Try again.",
    );
    expect(screen.queryByText("SECRET database connection detail")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("retains existing rows while exposing a refresh error and retry", () => {
    render(
      <SubmissionHistory
        submissions={[newestSubmission, olderSubmission]}
        loading={false}
        error="refresh failed"
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByRole("alert")).toBeVisible();
    expect(screen.getByRole("table", { name: "Submission history" })).toBeVisible();
    expect(screen.getAllByTestId("submission-id")).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Retry" })).toBeVisible();
  });

  it("uses one responsive table row set with mobile data labels", () => {
    const { container } = render(
      <SubmissionHistory
        submissions={[newestSubmission, olderSubmission]}
        loading={false}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(container.querySelectorAll("table")).toHaveLength(1);
    expect(container.querySelectorAll("tbody .submission-history__row")).toHaveLength(2);
    expect(container.querySelectorAll("ul, ol")).toHaveLength(0);
    for (const row of container.querySelectorAll("tbody .submission-history__row")) {
      expect(Array.from(row.querySelectorAll("td")).map(cell => cell.getAttribute("data-label")))
        .toEqual(["Submission", "Problem", "Language", "Status", "Submitted", "Duration"]);
    }

    const componentCss = container.querySelector("style")?.textContent ?? "";
    expect(componentCss).toContain("@media (max-width: 768px)");
    expect(componentCss).toContain("overflow-wrap");
    expect(componentCss).toContain("table-layout: fixed");
  });

  it("uses compact semantic rows at 721px for the longest status and all metadata", () => {
    const timedOutSubmission: Submission = {
      id: "submission-with-a-long-identifier-that-must-wrap",
      problemId: "slow-solution",
      language: "cpp",
      status: "time_limit_exceeded",
      result: { status: "time_limit_exceeded", durationMs: 1_234 },
      createdAt: "2026-06-30T11:22:33Z",
      updatedAt: "2026-06-30T11:22:35Z",
    };
    const { container } = render(
      <SubmissionHistory
        submissions={[timedOutSubmission]}
        loading={false}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    const table = screen.getByRole("table", { name: "Submission history" });
    const rows = within(table).getAllByRole("row");
    expect(container.querySelectorAll("table")).toHaveLength(1);
    expect(rows).toHaveLength(2);
    expect(container.querySelectorAll("tbody .submission-history__row")).toHaveLength(1);
    expect(container.querySelectorAll("ul, ol")).toHaveLength(0);

    const dataRow = rows[1];
    expect(within(dataRow).getByTestId("submission-id")).toHaveTextContent(
      timedOutSubmission.id,
    );
    expect(within(dataRow).getByText("slow-solution")).toBeVisible();
    expect(within(dataRow).getByText("C++")).toBeVisible();
    expect(within(dataRow).getByText("Time Limit Exceeded")).toBeVisible();
    expect(within(dataRow).getByText("1234 ms")).toBeVisible();
    expect(dataRow.querySelector("time")).toHaveAttribute(
      "datetime",
      timedOutSubmission.createdAt,
    );

    const componentCss = container.querySelector("style")?.textContent ?? "";
    const compactBreakpoint = componentCss.match(
      /@media \(max-width: (\d+)px\)\s*{[\s\S]*?\.submission-history__row\s*{[\s\S]*?display: grid;/,
    );
    expect(compactBreakpoint).not.toBeNull();
    expect(Number(compactBreakpoint?.[1])).toBeGreaterThanOrEqual(721);
  });
});

describe("SubmissionsPage", () => {
  let SubmissionsPage: ComponentType;
  let latestHistoryProps: SubmissionHistoryProps | null = null;
  let historyRenderCount = 0;

  beforeAll(async () => {
    vi.doMock("@/components/submission-history", () => ({
      SubmissionHistory: (props: SubmissionHistoryProps) => {
        latestHistoryProps = props;
        historyRenderCount += 1;
        return <SubmissionHistory {...props} />;
      },
    }));
    SubmissionsPage = (await import("@/app/submissions/page")).default;
  });

  afterAll(() => {
    vi.doUnmock("@/components/submission-history");
  });

  beforeEach(() => {
    api.getSubmissions.mockReset();
    latestHistoryProps = null;
    historyRenderCount = 0;
  });

  it("loads submissions once through the API and renders the returned order", async () => {
    const request = deferred<Submission[]>();
    api.getSubmissions.mockReturnValue(request.promise);

    render(<SubmissionsPage />);

    expect(api.getSubmissions.mock.calls).toEqual([[]]);
    expect(screen.getByRole("status", { name: "Loading submissions" })).toBeVisible();

    await act(async () => request.resolve([newestSubmission, olderSubmission]));

    expect(screen.getAllByTestId("submission-id").map(cell => cell.textContent)).toEqual([
      newestSubmission.id,
      olderSubmission.id,
    ]);
    expect(screen.queryByRole("status", { name: "Loading submissions" }))
      .not.toBeInTheDocument();
  });

  it("renders a generic retryable error without leaking API details", async () => {
    api.getSubmissions.mockRejectedValue(
      new Error("SECRET upstream database address"),
    );

    render(<SubmissionsPage />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Unable to load submissions. Try again.",
    );
    expect(screen.queryByText("SECRET upstream database address")).not.toBeInTheDocument();
    expect(api.getSubmissions.mock.calls).toEqual([[]]);
  });

  it("starts a fresh request on Retry and clears the error while loading", async () => {
    const retryRequest = deferred<Submission[]>();
    api.getSubmissions
      .mockRejectedValueOnce(new Error("offline"))
      .mockReturnValueOnce(retryRequest.promise);
    const user = userEvent.setup();

    render(<SubmissionsPage />);
    await user.click(await screen.findByRole("button", { name: "Retry" }));

    expect(api.getSubmissions.mock.calls).toEqual([[], []]);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByRole("status", { name: "Loading submissions" })).toBeVisible();

    await act(async () => retryRequest.resolve([runningSubmission]));
    expect(screen.getByText(runningSubmission.id)).toBeVisible();
  });

  it("retains prior rows through a failed refresh and its retry", async () => {
    const retryRequest = deferred<Submission[]>();
    api.getSubmissions
      .mockResolvedValueOnce([newestSubmission, olderSubmission])
      .mockRejectedValueOnce(new Error("refresh failed"))
      .mockReturnValueOnce(retryRequest.promise);
    const user = userEvent.setup();

    render(<SubmissionsPage />);
    expect(await screen.findByText(newestSubmission.id)).toBeVisible();

    act(() => latestHistoryProps?.onRetry());
    expect(await screen.findByRole("alert")).toBeVisible();
    expect(screen.getAllByTestId("submission-id")).toHaveLength(2);

    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getAllByTestId("submission-id")).toHaveLength(2);
    expect(screen.getByRole("status")).toHaveTextContent("Refreshing submissions");

    await act(async () => retryRequest.resolve([runningSubmission]));
    expect(screen.getAllByTestId("submission-id")).toHaveLength(1);
    expect(screen.getByText(runningSubmission.id)).toBeVisible();
  });

  it("lets the newest retry win when requests settle out of order", async () => {
    const staleRetry = deferred<Submission[]>();
    const currentRetry = deferred<Submission[]>();
    api.getSubmissions
      .mockResolvedValueOnce([newestSubmission])
      .mockReturnValueOnce(staleRetry.promise)
      .mockReturnValueOnce(currentRetry.promise);

    render(<SubmissionsPage />);
    expect(await screen.findByText(newestSubmission.id)).toBeVisible();

    act(() => latestHistoryProps?.onRetry());
    await waitFor(() => expect(api.getSubmissions).toHaveBeenCalledTimes(2));
    act(() => latestHistoryProps?.onRetry());
    await waitFor(() => expect(api.getSubmissions).toHaveBeenCalledTimes(3));

    await act(async () => currentRetry.resolve([runningSubmission]));
    expect(screen.getByText(runningSubmission.id)).toBeVisible();

    await act(async () => staleRetry.resolve([olderSubmission]));
    expect(screen.getByText(runningSubmission.id)).toBeVisible();
    expect(screen.queryByText(olderSubmission.id)).not.toBeInTheDocument();
    expect(api.getSubmissions.mock.calls).toEqual([[], [], []]);
  });

  it("does not update after unmount", async () => {
    const request = deferred<Submission[]>();
    api.getSubmissions.mockReturnValue(request.promise);
    const view = render(<SubmissionsPage />);
    const renderCountBeforeUnmount = historyRenderCount;

    view.unmount();
    await act(async () => request.resolve([newestSubmission]));

    expect(historyRenderCount).toBe(renderCountBeforeUnmount);
    expect(api.getSubmissions).toHaveBeenCalledTimes(1);
  });

  it("shares only the in-flight initial request during StrictMode replay", async () => {
    const strictRequest = deferred<Submission[]>();
    const laterMountRequest = deferred<Submission[]>();
    api.getSubmissions
      .mockReturnValueOnce(strictRequest.promise)
      .mockReturnValueOnce(laterMountRequest.promise);

    const firstView = render(
      <StrictMode>
        <SubmissionsPage />
      </StrictMode>,
    );

    expect(api.getSubmissions).toHaveBeenCalledTimes(1);
    await act(async () => strictRequest.resolve([newestSubmission]));
    expect(screen.getByText(newestSubmission.id)).toBeVisible();

    firstView.unmount();
    render(<SubmissionsPage />);
    expect(api.getSubmissions).toHaveBeenCalledTimes(2);

    await act(async () => laterMountRequest.resolve([]));
    expect(screen.getByText("No submissions yet")).toBeVisible();
    expect(api.getSubmissions.mock.calls).toEqual([[], []]);
  });
});
