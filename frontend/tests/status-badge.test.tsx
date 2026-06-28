import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatusBadge } from "@/components/status-badge";
import { statusMeta } from "@/lib/judge";
import type { SubmissionStatus } from "@/lib/types";

const expectedStatuses = [
  ["queued", "Queued"],
  ["running", "Running"],
  ["accepted", "Accepted"],
  ["wrong_answer", "Wrong Answer"],
  ["runtime_error", "Runtime Error"],
  ["time_limit_exceeded", "Time Limit Exceeded"],
  ["internal_error", "Internal Error"],
] as const satisfies ReadonlyArray<readonly [SubmissionStatus, string]>;

describe("StatusBadge", () => {
  it("covers every status in the judge metadata", () => {
    expect(expectedStatuses.map(([status]) => status)).toEqual(Object.keys(statusMeta));
  });

  it.each(expectedStatuses)("renders the canonical %s presentation", (status, label) => {
    render(<StatusBadge status={status} />);

    const visibleLabel = screen.getByText(label);
    const icon = visibleLabel.parentElement?.querySelector("svg");

    expect(visibleLabel).toHaveAttribute("data-status", status);
    expect(icon).toHaveAttribute("aria-hidden", "true");
  });
});
