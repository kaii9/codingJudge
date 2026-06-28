"use client";

import { RotateCcw } from "lucide-react";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { CodeWorkspace } from "@/components/code-workspace";
import { JudgeResultPanel } from "@/components/judge-result-panel";
import { ProblemRail } from "@/components/problem-rail";
import { ProblemStatement } from "@/components/problem-statement";
import { useSubmissionPolling } from "@/hooks/use-submission-polling";
import {
  createSubmission,
  getProblem,
  getProblems,
  getSubmissions,
} from "@/lib/api";
import { isTerminalStatus } from "@/lib/judge";
import type {
  CreateSubmissionInput,
  Problem,
  Submission,
} from "@/lib/types";

type MobileTab = "problem" | "code" | "result";

type LoadState =
  | { kind: "loading"; problemId: string; loadVersion: number; runId: number }
  | { kind: "unknown"; problemId: string; loadVersion: number; runId: number }
  | { kind: "error"; problemId: string; loadVersion: number; runId: number }
  | {
      kind: "ready";
      problemId: string;
      loadVersion: number;
      runId: number;
      problems: Problem[];
      problem: Problem;
      submissions: Submission[];
    };

interface SubmissionState {
  problemId: string;
  runId: number;
  active: Submission | null;
  submitting: boolean;
  error: string | null;
}

interface TabState {
  problemId: string;
  runId: number;
  active: MobileTab;
}

interface WorkbenchProps {
  problemId: string;
}

const RECENT_SUBMISSION_LIMIT = 8;

const workbenchCss = `
  .workbench {
    min-height: 42rem;
    height: calc(100vh - 52px);
    display: grid;
    grid-template-columns: minmax(13rem, 18rem) minmax(20rem, 1fr) minmax(24rem, 40%);
    grid-template-rows: auto minmax(24rem, 1fr) minmax(18rem, 36%);
    background: var(--color-gray-50);
  }
  .workbench__tabs {
    grid-column: 2 / 4;
    display: flex;
    gap: 2px;
    min-height: 2.75rem;
    padding: 0.25rem;
    background: var(--color-gray-100);
    border-bottom: 1px solid var(--color-border);
  }
  .workbench__tabs button {
    min-width: 6rem;
    min-height: 2.25rem;
    padding: 0 0.75rem;
    color: var(--color-gray-700);
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--radius-control);
    font: inherit;
    font-size: 0.8125rem;
    font-weight: 700;
    cursor: pointer;
  }
  .workbench__tabs button[aria-selected="true"] {
    color: var(--color-navy-950);
    background: var(--color-white);
    border-color: var(--color-border);
  }
  .workbench__rail {
    min-width: 0;
    grid-column: 1;
    grid-row: 1 / 4;
    overflow: auto;
    padding: 1rem;
    background: var(--color-white);
    border-right: 1px solid var(--color-border);
  }
  .workbench__problem,
  .workbench__code,
  .workbench__result {
    min-width: 0;
    min-height: 0;
    overflow: auto;
  }
  .workbench__problem {
    grid-column: 2;
    grid-row: 2;
    padding: 1.25rem;
    background: var(--color-white);
    border-right: 1px solid var(--color-border);
  }
  .workbench__code {
    grid-column: 3;
    grid-row: 2;
    padding: 1rem;
    background: var(--color-gray-50);
  }
  .workbench__result {
    grid-column: 2 / 4;
    grid-row: 3;
  }
  .workbench__submit-error {
    padding: 0.75rem 1rem;
    color: #a3262f;
    background: #fdecee;
    border-top: 1px solid #ecb6bb;
  }
  .workbench-state {
    min-height: 42rem;
    display: grid;
    align-content: center;
    justify-items: center;
    gap: 0.75rem;
    padding: 2rem;
    text-align: center;
  }
  .workbench-state button {
    min-height: 2.5rem;
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0 0.875rem;
    color: var(--color-white);
    background: var(--color-navy-900);
    border: 0;
    border-radius: var(--radius-control);
    font: inherit;
    font-weight: 700;
    cursor: pointer;
  }
  @media (max-width: 900px) {
    .workbench {
      height: auto;
      min-height: calc(100vh - 56px);
      grid-template-columns: minmax(0, 1fr);
      grid-template-rows: auto minmax(36rem, auto);
    }
    .workbench__tabs {
      position: sticky;
      z-index: 10;
      top: 56px;
      grid-column: 1;
      grid-row: 1;
    }
    .workbench__tabs button {
      min-width: 0;
      flex: 1;
    }
    .workbench__rail {
      display: none;
    }
    .workbench__problem,
    .workbench__code,
    .workbench__result {
      grid-column: 1;
      grid-row: 2;
      border-right: 0;
    }
    .workbench [role="tabpanel"][data-active="false"] {
      display: none;
    }
  }
`;

function isNotFound(error: unknown) {
  return (
    typeof error === "object"
    && error !== null
    && "status" in error
    && error.status === 404
  );
}

function WorkbenchState({
  kind,
  onRetry,
}: {
  kind: "loading" | "unknown" | "error";
  onRetry: () => void;
}) {
  if (kind === "loading") {
    return (
      <>
        <style>{workbenchCss}</style>
        <main className="workbench-state" aria-busy="true" style={{ minHeight: "42rem" }}>
          <h1>Loading workbench</h1>
          <p>Loading the selected problem and submissions.</p>
        </main>
      </>
    );
  }

  if (kind === "unknown") {
    return (
      <>
        <style>{workbenchCss}</style>
        <main className="workbench-state" style={{ minHeight: "42rem" }}>
          <h1>Problem not found</h1>
          <p>This problem is unavailable or does not exist.</p>
        </main>
      </>
    );
  }

  return (
    <>
      <style>{workbenchCss}</style>
      <main className="workbench-state" style={{ minHeight: "42rem" }}>
        <h1>Workbench unavailable</h1>
        <p>Unable to load the problem workspace. Try again.</p>
        <button type="button" onClick={onRetry}>
          <RotateCcw size={16} aria-hidden="true" />
          <span>Retry loading</span>
        </button>
      </main>
    </>
  );
}

export function Workbench({ problemId }: WorkbenchProps) {
  const [loadState, setLoadState] = useState<LoadState>(() => ({
    kind: "loading",
    problemId,
    loadVersion: 0,
    runId: 0,
  }));
  const [loadVersion, setLoadVersion] = useState(0);
  const [submissionState, setSubmissionState] = useState<SubmissionState | null>(null);
  const [tabState, setTabState] = useState<TabState | null>(null);
  const mountedRef = useRef(false);
  const loadRunRef = useRef(0);
  const createRunRef = useRef(0);
  const refreshedSubmissionIdsRef = useRef(new Set<string>());

  const currentLoadState = loadState.problemId === problemId
    && loadState.loadVersion === loadVersion
    ? loadState
    : { kind: "loading" as const, problemId, loadVersion, runId: 0 };
  const activeLoadRun = currentLoadState.kind === "ready"
    ? currentLoadState.runId
    : null;
  const currentSubmissionState = submissionState?.problemId === problemId
    && submissionState.runId === activeLoadRun
    ? submissionState
    : null;
  const activeSubmissionId = currentSubmissionState?.active?.id ?? null;
  const polling = useSubmissionPolling(activeSubmissionId);
  const polledSubmission = polling.submission?.id === activeSubmissionId
    ? polling.submission
    : null;
  const latestSubmission = polledSubmission
    ?? currentSubmissionState?.active
    ?? null;
  const mobileTab = tabState?.problemId === problemId
    && tabState.runId === activeLoadRun
    ? tabState.active
    : "problem";

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      createRunRef.current += 1;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    const runId = loadRunRef.current + 1;
    loadRunRef.current = runId;

    const load = async () => {
      try {
        const [problems, problem, submissions] = await Promise.all([
          getProblems(),
          getProblem(problemId),
          getSubmissions(),
        ]);
        if (cancelled) return;

        setLoadState({
          kind: "ready",
          problemId,
          loadVersion,
          runId,
          problems,
          problem,
          submissions,
        });
      } catch (error) {
        if (cancelled) return;
        setLoadState({
          kind: isNotFound(error) ? "unknown" : "error",
          problemId,
          loadVersion,
          runId,
        });
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [loadVersion, problemId]);

  const retryLoad = useCallback(() => {
    setLoadVersion(version => version + 1);
  }, []);

  const handleSubmit = useCallback(async (
    input: Pick<CreateSubmissionInput, "language" | "code">,
  ) => {
    if (activeLoadRun === null) return;

    const submittedProblemId = problemId;
    const submittedLoadRun = activeLoadRun;
    const createRun = createRunRef.current + 1;
    createRunRef.current = createRun;
    setSubmissionState(current => ({
      problemId: submittedProblemId,
      runId: submittedLoadRun,
      active: current?.problemId === submittedProblemId
        && current.runId === submittedLoadRun
        ? current.active
        : null,
      submitting: true,
      error: null,
    }));

    try {
      const submission = await createSubmission({
        problemId: submittedProblemId,
        language: input.language,
        code: input.code,
      });
      if (
        !mountedRef.current
        || loadRunRef.current !== submittedLoadRun
        || createRunRef.current !== createRun
      ) return;

      setSubmissionState({
        problemId: submittedProblemId,
        runId: submittedLoadRun,
        active: submission,
        submitting: false,
        error: null,
      });
      setTabState({
        problemId: submittedProblemId,
        runId: submittedLoadRun,
        active: "result",
      });
    } catch {
      if (
        mountedRef.current
        && loadRunRef.current === submittedLoadRun
        && createRunRef.current === createRun
      ) {
        setSubmissionState(current => ({
          problemId: submittedProblemId,
          runId: submittedLoadRun,
          active: current?.problemId === submittedProblemId
            && current.runId === submittedLoadRun
            ? current.active
            : null,
          submitting: false,
          error: "Submission could not be created. Try again.",
        }));
      }
    } finally {
      if (
        mountedRef.current
        && loadRunRef.current === submittedLoadRun
        && createRunRef.current === createRun
      ) {
        setSubmissionState(current => current?.problemId === submittedProblemId
          && current.runId === submittedLoadRun
          ? { ...current, submitting: false }
          : current);
      }
    }
  }, [activeLoadRun, problemId]);

  const terminalSubmissionId = latestSubmission
    && activeSubmissionId === latestSubmission.id
    && isTerminalStatus(latestSubmission.status)
    ? latestSubmission.id
    : null;

  useEffect(() => {
    if (
      !terminalSubmissionId
      || refreshedSubmissionIdsRef.current.has(terminalSubmissionId)
    ) return;

    refreshedSubmissionIdsRef.current.add(terminalSubmissionId);
    if (activeLoadRun !== null) {
      void getSubmissions().then(submissions => {
        if (!mountedRef.current) return;

        setLoadState(current => current.kind === "ready"
          && current.problemId === problemId
          && current.runId === activeLoadRun
          ? { ...current, submissions }
          : current);
      }).catch(() => {
        // The current result remains useful when a nonessential history refresh fails.
      });
    }
  }, [activeLoadRun, problemId, terminalSubmissionId]);

  if (currentLoadState.kind !== "ready") {
    return <WorkbenchState kind={currentLoadState.kind} onRetry={retryLoad} />;
  }

  const tabs: ReadonlyArray<{ id: MobileTab; label: string }> = [
    { id: "problem", label: "Problem" },
    { id: "code", label: "Code" },
    { id: "result", label: "Result" },
  ];

  return (
    <main className="workbench">
      <style>{workbenchCss}</style>

      <div className="workbench__tabs" role="tablist" aria-label="Workbench views">
        {tabs.map(tab => (
          <button
            key={tab.id}
            id={`workbench-${tab.id}-tab`}
            type="button"
            role="tab"
            aria-controls={`workbench-${tab.id}-panel`}
            aria-selected={mobileTab === tab.id}
            tabIndex={mobileTab === tab.id ? 0 : -1}
            onClick={() => setTabState({
              problemId,
              runId: currentLoadState.runId,
              active: tab.id,
            })}
          >
            {tab.label}
          </button>
        ))}
      </div>

      <div className="workbench__rail">
        <ProblemRail
          problems={currentLoadState.problems}
          activeProblemId={problemId}
          recentSubmissions={currentLoadState.submissions.slice(0, RECENT_SUBMISSION_LIMIT)}
        />
      </div>

      <div
        id="workbench-problem-panel"
        className="workbench__problem"
        role="tabpanel"
        aria-labelledby="workbench-problem-tab"
        data-active={mobileTab === "problem"}
      >
        <ProblemStatement problem={currentLoadState.problem} />
      </div>

      <div
        id="workbench-code-panel"
        className="workbench__code"
        role="tabpanel"
        aria-labelledby="workbench-code-tab"
        data-active={mobileTab === "code"}
      >
        {currentSubmissionState?.error ? (
          <p
            className="workbench__submit-error"
            role="alert"
            aria-label="Submission error"
          >
            {currentSubmissionState.error}
          </p>
        ) : null}
        <CodeWorkspace
          problemId={problemId}
          submitting={currentSubmissionState?.submitting ?? false}
          onSubmit={handleSubmit}
        />
      </div>

      <div
        id="workbench-result-panel"
        className="workbench__result"
        role="tabpanel"
        aria-labelledby="workbench-result-tab"
        data-active={mobileTab === "result"}
      >
        <JudgeResultPanel
          submission={latestSubmission}
          error={polling.error}
          onRetry={polling.retry}
        />
      </div>
    </main>
  );
}
