import {
  CircleCheck,
  CircleEllipsis,
  CircleX,
  LoaderCircle,
  ServerCrash,
  TimerOff,
  TriangleAlert,
  type LucideIcon,
} from "lucide-react";
import { statusMeta } from "@/lib/judge";
import type { SubmissionStatus } from "@/lib/types";

const statusIcons: Record<SubmissionStatus, LucideIcon> = {
  queued: CircleEllipsis,
  running: LoaderCircle,
  accepted: CircleCheck,
  wrong_answer: CircleX,
  runtime_error: TriangleAlert,
  time_limit_exceeded: TimerOff,
  internal_error: ServerCrash,
};

interface StatusBadgeProps {
  status: SubmissionStatus;
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const { label, variant } = statusMeta[status];
  const Icon = statusIcons[status];

  return (
    <span className="status-badge" data-variant={variant}>
      <Icon aria-hidden="true" focusable="false" size={14} strokeWidth={2.25} />
      <span data-status={status}>{label}</span>
    </span>
  );
}
