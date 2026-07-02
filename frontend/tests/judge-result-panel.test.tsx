import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { JudgeResultPanel } from "@/components/judge-result-panel";
import type { Submission, SubmissionStatus } from "@/lib/types";

const baseSubmission: Submission = {
  id: "sub-1",
  problemId: "sum",
  language: "python",
  status: "queued",
  createdAt: "2026-06-27T00:00:00Z",
  updatedAt: "2026-06-27T00:00:00Z",
};

function submissionWithStatus(status: SubmissionStatus): Submission {
  return {
    ...baseSubmission,
    status,
    result: {
      status,
      stdout: "3\n",
      stderr: "diagnostic\n",
      exitCode: 1,
      durationMs: 384,
    },
  };
}

afterEach(cleanup);

describe("JudgeResultPanel", () => {
  it.each([
    ["queued", "Queued"],
    ["running", "Running"],
    ["accepted", "Accepted"],
    ["wrong_answer", "Wrong Answer"],
    ["runtime_error", "Runtime Error"],
    ["time_limit_exceeded", "Time Limit Exceeded"],
    ["internal_error", "Internal Error"],
  ] as const)("renders the canonical %s status", (status, label) => {
    render(
      <JudgeResultPanel
        submission={submissionWithStatus(status)}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    const result = screen.getByRole("region", { name: "Judge result" });
    expect(within(result).getByText(label)).toHaveAttribute("data-status", status);
    expect(within(result).getByRole("status")).toHaveTextContent(`Submission status: ${label}`);
  });

  it("shows present result metadata and preserves output whitespace as text", () => {
    const stdout = "  first line\nsecond <script>alert(1)</script>\n";
    const stderr = "warning\n  detail\n";

    render(
      <JudgeResultPanel
        submission={{
          ...baseSubmission,
          status: "runtime_error",
          result: {
            status: "runtime_error",
            durationMs: 0,
            exitCode: 0,
            stdout,
            stderr,
          },
        }}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByText("0 ms")).toBeVisible();
    expect(screen.getByText("0", { selector: "dd" })).toBeVisible();
    expect(screen.getByLabelText("Standard output")).toHaveTextContent(stdout, {
      normalizeWhitespace: false,
    });
    expect(screen.getByLabelText("Standard error")).toHaveTextContent(stderr, {
      normalizeWhitespace: false,
    });
    expect(screen.queryByRole("script")).not.toBeInTheDocument();
  });

  it("distinguishes an explicitly empty stdout from an absent result", () => {
    const view = render(
      <JudgeResultPanel
        submission={{
          ...baseSubmission,
          status: "accepted",
          result: { status: "accepted", stdout: "" },
        }}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Output" })).toBeVisible();
    expect(screen.getByText("Output is empty.")).toBeVisible();
    expect(screen.getByLabelText("Standard output")).toBeEmptyDOMElement();

    view.rerender(
      <JudgeResultPanel
        submission={{ ...baseSubmission, status: "accepted" }}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.queryByRole("heading", { name: "Output" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Standard output")).not.toBeInTheDocument();
    expect(screen.queryByText("Output is empty.")).not.toBeInTheDocument();
  });

  it("omits result fields that are not present", () => {
    render(
      <JudgeResultPanel
        submission={{
          ...baseSubmission,
          status: "accepted",
          result: { status: "accepted" },
        }}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.queryByText("Duration")).not.toBeInTheDocument();
    expect(screen.queryByText("Exit code")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Output" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Error output" })).not.toBeInTheDocument();
  });

  it("never renders hidden inputs, expected output, or submitted source", () => {
    const privateSubmission = {
      ...submissionWithStatus("wrong_answer"),
      hiddenInput: "PRIVATE INPUT SENTINEL",
      expectedOutput: "PRIVATE EXPECTED SENTINEL",
      source: "PRIVATE SOURCE SENTINEL",
    };

    render(
      <JudgeResultPanel
        submission={privateSubmission}
        error={null}
        onRetry={vi.fn()}
      />,
    );

    expect(screen.queryByText("PRIVATE INPUT SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE EXPECTED SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE SOURCE SENTINEL")).not.toBeInTheDocument();
  });

  it("renders a stable empty state before the first submission", () => {
    render(
      <JudgeResultPanel submission={null} error={null} onRetry={vi.fn()} />,
    );

    const result = screen.getByRole("region", { name: "Judge result" });
    expect(result).toHaveStyle({ minHeight: "18rem" });
    expect(within(result).getByText("Submit code to see the judge result.")).toBeVisible();
    expect(within(result).queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
  });

  it("retains the latest submission and gates retry to polling errors", async () => {
    const user = userEvent.setup();
    const onRetry = vi.fn();
    const view = render(
      <JudgeResultPanel
        submission={submissionWithStatus("running")}
        error="SECRET transport detail"
        onRetry={onRetry}
      />,
    );

    expect(screen.getByText("Running")).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent("Result updates are unavailable.");
    expect(screen.queryByText("SECRET transport detail")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledTimes(1);

    view.rerender(
      <JudgeResultPanel
        submission={submissionWithStatus("running")}
        error={null}
        onRetry={onRetry}
      />,
    );

    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
  });
});
