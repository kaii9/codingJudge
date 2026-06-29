"use client";

import Link from "next/link";
import { RotateCcw } from "lucide-react";
import { StatusBadge } from "@/components/status-badge";
import type { Language, Submission } from "@/lib/types";

export interface SubmissionHistoryProps {
  submissions: readonly Submission[];
  loading: boolean;
  error: string | null;
  onRetry: () => void;
}

const languageLabels = {
  go: "Go",
  cpp: "C++",
  python: "Python",
} satisfies Record<Language, string>;

const timestampFormatter = new Intl.DateTimeFormat("en-US", {
  year: "numeric",
  month: "short",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hour12: false,
  timeZone: "UTC",
  timeZoneName: "short",
});

const submissionHistoryCss = `
  .submission-history {
    width: 100%;
    min-width: 0;
    max-width: 80rem;
    padding: 1.5rem;
    margin: 0 auto;
  }
  .submission-history__header {
    margin-bottom: 1rem;
  }
  .submission-history__header h1 {
    color: var(--color-navy-950);
    font-size: 1.5rem;
    line-height: 1.25;
  }
  .submission-history__error {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.75rem 1rem;
    margin-bottom: 1rem;
    color: #a3262f;
    background: #fdecee;
    border: 1px solid #ecb6bb;
    border-radius: 6px;
  }
  .submission-history__retry {
    min-width: 5.75rem;
    min-height: 2.25rem;
    display: inline-flex;
    flex: 0 0 auto;
    align-items: center;
    justify-content: center;
    gap: 0.4rem;
    padding: 0 0.75rem;
    color: var(--color-white);
    background: var(--color-navy-900);
    border: 0;
    border-radius: var(--radius-control);
    font: inherit;
    font-size: 0.8125rem;
    font-weight: 700;
    cursor: pointer;
  }
  .submission-history__table-wrap {
    width: 100%;
    max-width: 100%;
    min-width: 0;
    overflow: hidden;
    background: var(--color-white);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-panel);
  }
  .submission-history__table {
    width: 100%;
    max-width: 100%;
    table-layout: fixed;
    border-collapse: collapse;
  }
  .submission-history__table caption {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
  .submission-history__table th,
  .submission-history__table td {
    min-width: 0;
    padding: 0.75rem;
    overflow-wrap: anywhere;
    word-break: break-word;
    text-align: left;
    vertical-align: middle;
    border-bottom: 1px solid var(--color-border);
  }
  .submission-history__table th {
    color: var(--color-gray-700);
    background: var(--color-gray-100);
    font-size: 0.75rem;
    font-weight: 750;
  }
  .submission-history__table td {
    color: var(--color-gray-900);
    font-size: 0.8125rem;
    line-height: 1.4;
  }
  .submission-history__table tbody tr:last-child td {
    border-bottom: 0;
  }
  .submission-history__table a {
    color: #2357a6;
    font-weight: 700;
    text-decoration: underline;
    text-decoration-thickness: 1px;
    text-underline-offset: 2px;
  }
  .submission-history__refresh-status {
    margin-bottom: 0.5rem;
    color: var(--color-gray-500);
    font-size: 0.8125rem;
  }
  .submission-history__loading {
    min-height: 24rem;
  }
  .submission-history__skeleton {
    min-height: 24rem;
    display: grid;
    grid-template-rows: repeat(4, 4.75rem);
    overflow: hidden;
    background: var(--color-white);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-panel);
  }
  .submission-history__skeleton-row {
    height: 4.75rem;
    background: var(--color-gray-100);
    border-bottom: 1px solid var(--color-white);
  }
  .submission-history__empty {
    min-height: 18rem;
    display: grid;
    place-items: center;
    padding: 2rem;
    color: var(--color-gray-500);
    background: var(--color-white);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-panel);
    text-align: center;
  }
  .submission-history__sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
  @media (max-width: 720px) {
    .submission-history {
      padding: 1rem 0.75rem;
    }
    .submission-history__error {
      align-items: flex-start;
    }
    .submission-history__table-wrap {
      overflow: visible;
      background: transparent;
      border: 0;
      border-radius: 0;
    }
    .submission-history__table,
    .submission-history__table tbody {
      display: block;
      width: 100%;
    }
    .submission-history__table thead {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip: rect(0, 0, 0, 0);
    }
    .submission-history__row {
      min-width: 0;
      display: grid;
      gap: 0;
      margin-bottom: 0.75rem;
      overflow: hidden;
      background: var(--color-white);
      border: 1px solid var(--color-border);
      border-radius: 8px;
    }
    .submission-history__table td {
      min-width: 0;
      display: grid;
      grid-template-columns: minmax(5.75rem, 34%) minmax(0, 1fr);
      gap: 0.75rem;
      padding: 0.625rem 0.75rem;
      border-bottom: 1px solid var(--color-gray-100);
    }
    .submission-history__table td::before {
      content: attr(data-label);
      color: var(--color-gray-500);
      font-size: 0.75rem;
      font-weight: 700;
    }
  }
`;

function formatTimestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : timestampFormatter.format(date);
}

function HistoryError({ onRetry }: Pick<SubmissionHistoryProps, "onRetry">) {
  return (
    <div className="submission-history__error" role="alert">
      <p>Unable to load submissions. Try again.</p>
      <button className="submission-history__retry" type="button" onClick={onRetry}>
        <RotateCcw size={15} aria-hidden="true" focusable="false" />
        <span>Retry</span>
      </button>
    </div>
  );
}

function LoadingHistory() {
  return (
    <div className="submission-history__loading">
      <p
        className="submission-history__sr-only"
        role="status"
        aria-label="Loading submissions"
        aria-live="polite"
      >
        Loading submissions
      </p>
      <div className="submission-history__skeleton" aria-hidden="true">
        {Array.from({ length: 4 }, (_, index) => (
          <div className="submission-history__skeleton-row" key={index} />
        ))}
      </div>
    </div>
  );
}

function HistoryTable({ submissions }: Pick<SubmissionHistoryProps, "submissions">) {
  return (
    <div className="submission-history__table-wrap">
      <table className="submission-history__table">
        <caption>Submission history</caption>
        <colgroup>
          <col style={{ width: "17%" }} />
          <col style={{ width: "18%" }} />
          <col style={{ width: "11%" }} />
          <col style={{ width: "18%" }} />
          <col style={{ width: "25%" }} />
          <col style={{ width: "11%" }} />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">Submission</th>
            <th scope="col">Problem</th>
            <th scope="col">Language</th>
            <th scope="col">Status</th>
            <th scope="col">Submitted</th>
            <th scope="col">Duration</th>
          </tr>
        </thead>
        <tbody>
          {submissions.map(submission => {
            const durationMs = submission.result?.durationMs;

            return (
              <tr className="submission-history__row" key={submission.id}>
                <td data-label="Submission" data-testid="submission-id">
                  {submission.id}
                </td>
                <td data-label="Problem">
                  <Link href={`/problems/${encodeURIComponent(submission.problemId)}`}>
                    {submission.problemId}
                  </Link>
                </td>
                <td data-label="Language">{languageLabels[submission.language]}</td>
                <td data-label="Status"><StatusBadge status={submission.status} /></td>
                <td data-label="Submitted">
                  <time dateTime={submission.createdAt}>
                    {formatTimestamp(submission.createdAt)}
                  </time>
                </td>
                <td data-label="Duration">
                  {durationMs !== undefined ? `${durationMs} ms` : null}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export function SubmissionHistory({
  submissions,
  loading,
  error,
  onRetry,
}: SubmissionHistoryProps) {
  const hasSubmissions = submissions.length > 0;

  return (
    <section className="submission-history" aria-labelledby="submission-history-heading">
      <style>{submissionHistoryCss}</style>
      <header className="submission-history__header">
        <h1 id="submission-history-heading">Submissions</h1>
      </header>

      {error ? <HistoryError onRetry={onRetry} /> : null}
      {loading && hasSubmissions ? (
        <p
          className="submission-history__refresh-status"
          role="status"
          aria-live="polite"
        >
          Refreshing submissions
        </p>
      ) : null}

      {hasSubmissions ? <HistoryTable submissions={submissions} /> : null}
      {!hasSubmissions && loading ? <LoadingHistory /> : null}
      {!hasSubmissions && !loading && !error ? (
        <p className="submission-history__empty">No submissions yet</p>
      ) : null}
    </section>
  );
}
