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
import { WorkbenchTabs, type WorkbenchTab } from "@/components/workbench-tabs";
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
      warning: boolean;
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
  active: WorkbenchTab;
}

interface WorkbenchProps {
  problemId: string;
}

interface WorkspaceLoadRequests {
  problems: Promise<PromiseSettledResult<Problem[]>>;
  problem: Promise<PromiseSettledResult<Problem>>;
  submissions: Promise<PromiseSettledResult<Submission[]>>;
}

const RECENT_SUBMISSION_LIMIT = 8;
const inFlightWorkspaceLoads = new Map<string, WorkspaceLoadRequests>();

function settle<T>(request: Promise<T>): Promise<PromiseSettledResult<T>> {
  return request.then(
    value => ({ status: "fulfilled", value }),
    reason => ({ status: "rejected", reason }),
  );
}

function loadWorkspace(problemId: string): WorkspaceLoadRequests {
  const existing = inFlightWorkspaceLoads.get(problemId);
  if (existing) return existing;

  const requests: WorkspaceLoadRequests = {
    problems: settle(getProblems()),
    problem: settle(getProblem(problemId)),
    submissions: settle(getSubmissions()),
  };
  inFlightWorkspaceLoads.set(problemId, requests);
  void requests.problem.then(() => {
    if (inFlightWorkspaceLoads.get(problemId) === requests) {
      inFlightWorkspaceLoads.delete(problemId);
    }
  });
  return requests;
}

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
      <main className="workbench-state" aria-busy="true" style={{ minHeight: "42rem" }}>
        <h1>Loading workbench</h1>
        <p>Loading the selected problem and submissions.</p>
      </main>
    );
  }

  if (kind === "unknown") {
    return (
      <main className="workbench-state" style={{ minHeight: "42rem" }}>
        <h1>Problem not found</h1>
        <p>This problem is unavailable or does not exist.</p>
      </main>
    );
  }

  return (
    <main className="workbench-state" style={{ minHeight: "42rem" }}>
      <h1>Workbench unavailable</h1>
      <p>Unable to load the problem workspace. Try again.</p>
      <button type="button" onClick={onRetry}>
        <RotateCcw size={16} aria-hidden="true" />
        <span>Retry loading</span>
      </button>
    </main>
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
  const historyRefreshRunRef = useRef(0);
  const refreshedSubmissionIdsRef = useRef(new Set<string>());
  const pendingResultFocusRef = useRef(false);

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
      historyRefreshRunRef.current += 1;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    const runId = loadRunRef.current + 1;
    loadRunRef.current = runId;
    const historyRun = historyRefreshRunRef.current + 1;
    historyRefreshRunRef.current = historyRun;

    const load = async () => {
      const requests = loadWorkspace(problemId);
      const problemResult = await requests.problem;
      if (cancelled) return;

      if (problemResult.status === "rejected") {
        setLoadState({
          kind: isNotFound(problemResult.reason) ? "unknown" : "error",
          problemId,
          loadVersion,
          runId,
        });
        return;
      }

      setLoadState({
        kind: "ready",
        problemId,
        loadVersion,
        runId,
        problems: [problemResult.value],
        problem: problemResult.value,
        submissions: [],
        warning: false,
      });

      void requests.problems.then(result => {
        if (cancelled) return;
        setLoadState(current => current.kind === "ready"
          && current.problemId === problemId
          && current.runId === runId
          ? {
              ...current,
              problems: result.status === "fulfilled" ? result.value : current.problems,
              warning: current.warning || result.status === "rejected",
            }
          : current);
      });
      void requests.submissions.then(result => {
        if (
          cancelled
          || historyRefreshRunRef.current !== historyRun
        ) return;
        setLoadState(current => current.kind === "ready"
          && current.problemId === problemId
          && current.runId === runId
          ? {
              ...current,
              submissions: result.status === "fulfilled" ? result.value : current.submissions,
              warning: current.warning || result.status === "rejected",
            }
          : current);
      });
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
      pendingResultFocusRef.current = true;
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
        setTabState({
          problemId: submittedProblemId,
          runId: submittedLoadRun,
          active: "code",
        });
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
      const refreshRun = historyRefreshRunRef.current + 1;
      historyRefreshRunRef.current = refreshRun;
      void getSubmissions().then(submissions => {
        if (
          !mountedRef.current
          || historyRefreshRunRef.current !== refreshRun
        ) return;

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

  useEffect(() => {
    if (
      !pendingResultFocusRef.current
      || mobileTab !== "result"
      || activeSubmissionId === null
    ) return;

    pendingResultFocusRef.current = false;
    document.getElementById("workbench-result-panel")?.focus();
  }, [activeSubmissionId, mobileTab]);

  if (currentLoadState.kind !== "ready") {
    return <WorkbenchState kind={currentLoadState.kind} onRetry={retryLoad} />;
  }

  const handleTabChange = (active: WorkbenchTab) => {
    setTabState({
      problemId,
      runId: currentLoadState.runId,
      active,
    });
  };

  return (
    <main className="workbench">
      <WorkbenchTabs active={mobileTab} onChange={handleTabChange} />

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
        tabIndex={-1}
        data-active={mobileTab === "problem"}
      >
        {currentLoadState.warning ? (
          <p
            className="workbench__warning"
            role="status"
            aria-label="Workspace warning"
          >
            Some workspace data is unavailable.
          </p>
        ) : null}
        <ProblemStatement problem={currentLoadState.problem} />
      </div>

      <div
        id="workbench-code-panel"
        className="workbench__code"
        role="tabpanel"
        aria-labelledby="workbench-code-tab"
        tabIndex={-1}
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
        tabIndex={-1}
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
