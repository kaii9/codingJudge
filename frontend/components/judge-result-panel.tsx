import { RotateCcw } from "lucide-react";
import { StatusBadge } from "@/components/status-badge";
import type { Submission } from "@/lib/types";

interface JudgeResultPanelProps {
  submission: Submission | null;
  error: string | null;
  onRetry: () => void;
}

const panelStyle = {
  minHeight: "18rem",
  overflow: "auto",
  padding: "1rem",
  background: "var(--color-white)",
  borderTop: "1px solid var(--color-border)",
} as const;

const outputStyle = {
  overflowX: "auto",
  padding: "0.75rem",
  color: "var(--color-gray-100)",
  background: "var(--color-gray-900)",
  borderRadius: "var(--radius-control)",
  fontFamily: "var(--font-geist-mono), monospace",
  fontSize: "0.8125rem",
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
} as const;

export function JudgeResultPanel({
  submission,
  error,
  onRetry,
}: JudgeResultPanelProps) {
  const result = submission?.result;
  const hasMetadata = result?.durationMs !== undefined || result?.exitCode !== undefined;

  return (
    <section
      className="judge-result-panel"
      aria-labelledby="judge-result-heading"
      style={panelStyle}
    >
      <h2 id="judge-result-heading">Judge result</h2>

      {submission ? (
        <div role="status" aria-live="polite" aria-atomic="true">
          <span>Submission status: </span>
          <StatusBadge status={submission.status} />
        </div>
      ) : (
        <p>Submit code to see the judge result.</p>
      )}

      {error ? (
        <div role="alert">
          <p>Result updates are unavailable.</p>
          <button type="button" onClick={onRetry}>
            <RotateCcw size={16} aria-hidden="true" />
            <span>Retry</span>
          </button>
        </div>
      ) : null}

      {hasMetadata ? (
        <dl>
          {result?.durationMs !== undefined ? (
            <div>
              <dt>Duration</dt>
              <dd>{result.durationMs} ms</dd>
            </div>
          ) : null}
          {result?.exitCode !== undefined ? (
            <div>
              <dt>Exit code</dt>
              <dd>{result.exitCode}</dd>
            </div>
          ) : null}
        </dl>
      ) : null}

      {result?.stdout !== undefined ? (
        <section aria-labelledby="judge-output-heading">
          <h3 id="judge-output-heading">Output</h3>
          {result.stdout.length === 0 ? <p>Output is empty.</p> : null}
          <pre aria-label="Standard output" style={outputStyle}>{result.stdout}</pre>
        </section>
      ) : null}

      {result?.stderr !== undefined ? (
        <section aria-labelledby="judge-error-output-heading">
          <h3 id="judge-error-output-heading">Error output</h3>
          {result.stderr.length === 0 ? <p>Error output is empty.</p> : null}
          <pre aria-label="Standard error" style={outputStyle}>{result.stderr}</pre>
        </section>
      ) : null}
    </section>
  );
}
