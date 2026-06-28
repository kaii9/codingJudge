"use client";

import {
  useCallback,
  useEffect,
  useEffectEvent,
  useMemo,
  useState,
} from "react";
import { getSubmission } from "@/lib/api";
import { isTerminalStatus } from "@/lib/judge";
import type { Submission } from "@/lib/types";

type Loader = (id: string, signal: AbortSignal) => Promise<Submission>;

interface SubmissionTarget {
  id: string | null;
}

interface PollingRun {
  target: SubmissionTarget;
  intervalMs: number;
  retryVersion: number;
}

interface SubmissionState {
  target: SubmissionTarget;
  submission: Submission;
}

interface ErrorState {
  run: PollingRun;
  message: string;
}

export function useSubmissionPolling(
  id: string | null,
  load: Loader = getSubmission,
  intervalMs = 1000,
) {
  const [retryVersion, setRetryVersion] = useState(0);
  const [submissionState, setSubmissionState] = useState<SubmissionState | null>(null);
  const [errorState, setErrorState] = useState<ErrorState | null>(null);
  const [completedRun, setCompletedRun] = useState<PollingRun | null>(null);
  const target = useMemo<SubmissionTarget>(() => ({ id }), [id]);
  const run = useMemo<PollingRun>(() => ({
    target,
    intervalMs,
    retryVersion,
  }), [target, intervalMs, retryVersion]);
  const loadSubmission = useEffectEvent(load);

  const retry = useCallback(() => {
    setRetryVersion((version) => version + 1);
  }, []);

  useEffect(() => {
    const submissionId = run.target.id;
    if (!submissionId) return;

    let stopped = false;
    let timeout: ReturnType<typeof setTimeout> | undefined;
    let controller: AbortController | undefined;

    const poll = async () => {
      controller = new AbortController();
      try {
        const next = await loadSubmission(submissionId, controller.signal);
        if (stopped) return;

        setSubmissionState({ target: run.target, submission: next });
        if (isTerminalStatus(next.status)) {
          setCompletedRun(run);
        } else {
          timeout = setTimeout(poll, run.intervalMs);
        }
      } catch (caught) {
        if (
          stopped
          || (caught instanceof DOMException && caught.name === "AbortError")
        ) return;

        setErrorState({
          run,
          message: caught instanceof Error ? caught.message : "Polling failed",
        });
      }
    };

    void poll();
    return () => {
      stopped = true;
      if (timeout !== undefined) clearTimeout(timeout);
      controller?.abort();
    };
  }, [run]);

  const error = errorState?.run === run ? errorState.message : null;
  return {
    submission: submissionState?.target === target
      ? submissionState.submission
      : null,
    error,
    isPolling: Boolean(id) && completedRun !== run && error === null,
    retry,
  };
}
