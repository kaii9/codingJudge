import type { CreateSubmissionInput, Problem, Submission } from "@/lib/types";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, init);
  const payload = await response.json().catch(() => {
    if (response.ok) {
      throw new ApiError(
        response.status,
        "invalid_response",
        "API returned an invalid JSON response",
      );
    }
    return null;
  });
  if (!response.ok) {
    const body = payload as { error?: { code?: string; message?: string } } | null;
    throw new ApiError(
      response.status,
      body?.error?.code ?? "request_failed",
      body?.error?.message ?? `Request failed with status ${response.status}`,
    );
  }
  return payload as T;
}

export const getProblems = () => request<Problem[]>("/api/problems");
export const getProblem = (id: string) => request<Problem>(`/api/problems/${encodeURIComponent(id)}`);
export const getSubmissions = () => request<Submission[]>("/api/submissions");
export const getSubmission = (id: string, signal?: AbortSignal) =>
  request<Submission>(`/api/submissions/${encodeURIComponent(id)}`, { signal });
export const createSubmission = (input: CreateSubmissionInput) =>
  request<Submission>("/api/submissions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
