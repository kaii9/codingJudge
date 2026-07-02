import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatusBadge } from "@/components/status-badge";
import { statusMeta } from "@/lib/judge";
import type { StatusVariant } from "@/lib/judge";
import type { SubmissionStatus } from "@/lib/types";

const expectedStatuses = [
  ["queued", "Queued", "neutral", "lucide-circle-ellipsis"],
  ["running", "Running", "info", "lucide-loader-circle"],
  ["accepted", "Accepted", "success", "lucide-circle-check"],
  ["wrong_answer", "Wrong Answer", "warning", "lucide-circle-x"],
  ["runtime_error", "Runtime Error", "danger", "lucide-triangle-alert"],
  ["time_limit_exceeded", "Time Limit Exceeded", "danger", "lucide-timer-off"],
  ["internal_error", "Internal Error", "danger", "lucide-server-crash"],
] as const satisfies ReadonlyArray<
  readonly [SubmissionStatus, string, StatusVariant, string]
>;

describe("StatusBadge", () => {
  it("covers every status in the judge metadata", () => {
    expect(expectedStatuses.map(([status]) => status)).toEqual(Object.keys(statusMeta));
  });

  it.each(expectedStatuses)(
    "renders the canonical %s presentation",
    (status, label, variant, iconClass) => {
      render(<StatusBadge status={status} />);

      const visibleLabel = screen.getByText(label);
      const badge = visibleLabel.parentElement;
      const icon = badge?.querySelector("svg");

      expect(visibleLabel).toHaveAttribute("data-status", status);
      expect(badge).toHaveAttribute("data-variant", variant);
      expect(icon).toHaveClass(iconClass);
      expect(icon).toHaveAttribute("aria-hidden", "true");
      expect(icon).toHaveAttribute("focusable", "false");
    },
  );
});
