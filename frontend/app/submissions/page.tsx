"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { SubmissionHistory } from "@/components/submission-history";
import { getSubmissions } from "@/lib/api";
import type { Submission } from "@/lib/types";

const GENERIC_LOAD_ERROR = "Unable to load submissions. Try again.";
const REQUEST_DEADLINE_MS = 10_000;

let inFlightInitialRequest: Promise<Submission[]> | null = null;

function withRequestDeadline(request: Promise<Submission[]>) {
  return new Promise<Submission[]>((resolve, reject) => {
    const timeoutId = setTimeout(
      () => reject(new Error("Submission request timed out.")),
      REQUEST_DEADLINE_MS,
    );

    void request.then(
      submissions => {
        clearTimeout(timeoutId);
        resolve(submissions);
      },
      error => {
        clearTimeout(timeoutId);
        reject(error);
      },
    );
  });
}

function getInitialSubmissions() {
  if (inFlightInitialRequest) return inFlightInitialRequest;

  const request = withRequestDeadline(getSubmissions());
  inFlightInitialRequest = request;
  const clearRequest = () => {
    if (inFlightInitialRequest === request) inFlightInitialRequest = null;
  };
  void request.then(clearRequest, clearRequest);
  return request;
}

export default function SubmissionsPage() {
  const [submissions, setSubmissions] = useState<Submission[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(false);
  const requestVersionRef = useRef(0);

  const observeRequest = useCallback((request: Promise<Submission[]>) => {
    const requestVersion = requestVersionRef.current + 1;
    requestVersionRef.current = requestVersion;

    void request.then(
      nextSubmissions => {
        if (!mountedRef.current || requestVersionRef.current !== requestVersion) return;
        setSubmissions(nextSubmissions);
        setError(null);
        setLoading(false);
      },
      () => {
        if (!mountedRef.current || requestVersionRef.current !== requestVersion) return;
        setError(GENERIC_LOAD_ERROR);
        setLoading(false);
      },
    );
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    observeRequest(getInitialSubmissions());

    return () => {
      mountedRef.current = false;
      requestVersionRef.current += 1;
    };
  }, [observeRequest]);

  const retry = useCallback(() => {
    setError(null);
    setLoading(true);
    observeRequest(withRequestDeadline(getSubmissions()));
  }, [observeRequest]);

  return (
    <main style={{ width: "100%", minWidth: 0 }}>
      <SubmissionHistory
        submissions={submissions}
        loading={loading}
        error={error}
        onRetry={retry}
      />
    </main>
  );
}
