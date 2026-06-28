"use client";

import { useCallback, useEffect, useState } from "react";
import { getSubmission } from "@/lib/api";
import { isTerminalStatus } from "@/lib/judge";
import type { Submission } from "@/lib/types";

type Loader = (id: string, signal: AbortSignal) => Promise<Submission>;

interface PollingState {
  id: string | null;
  load: Loader;
  intervalMs: number;
  retryVersion: number;
  submission: Submission | null;
  error: string | null;
  isPolling: boolean;
}

export function useSubmissionPolling(
  id: string | null,
  load: Loader = getSubmission,
  intervalMs = 1000,
) {
  const [retryVersion, setRetryVersion] = useState(0);
  const [state, setState] = useState<PollingState>(() => ({
    id,
    load,
    intervalMs,
    retryVersion,
    submission: null,
    error: null,
    isPolling: Boolean(id),
  }));

  if (
    state.id !== id
    || state.load !== load
    || state.intervalMs !== intervalMs
    || state.retryVersion !== retryVersion
  ) {
    setState({
      id,
      load,
      intervalMs,
      retryVersion,
      submission: id ? state.submission : null,
      error: null,
      isPolling: Boolean(id),
    });
  }

  const retry = useCallback(() => {
    setRetryVersion((version) => version + 1);
  }, []);

  useEffect(() => {
    if (!id) return;

    let stopped = false;
    let timeout: ReturnType<typeof setTimeout> | undefined;
    let controller: AbortController | undefined;

    const poll = async () => {
      controller = new AbortController();
      try {
        const next = await load(id, controller.signal);
        if (stopped) return;

        const isTerminal = isTerminalStatus(next.status);
        setState((current) => ({
          ...current,
          submission: next,
          isPolling: !isTerminal,
        }));
        if (!isTerminal) {
          timeout = setTimeout(poll, intervalMs);
        }
      } catch (caught) {
        if (
          stopped
          || (caught instanceof DOMException && caught.name === "AbortError")
        ) return;

        setState((current) => ({
          ...current,
          error: caught instanceof Error ? caught.message : "Polling failed",
          isPolling: false,
        }));
      }
    };

    void poll();
    return () => {
      stopped = true;
      if (timeout !== undefined) clearTimeout(timeout);
      controller?.abort();
    };
  }, [id, intervalMs, load, retryVersion]);

  return {
    submission: state.submission,
    error: state.error,
    isPolling: state.isPolling,
    retry,
  };
}
