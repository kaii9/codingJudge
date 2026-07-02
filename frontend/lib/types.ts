export type Language = "go" | "cpp" | "python";
export type ProblemDifficulty = "easy" | "medium" | "hard";
export type ProblemCollection = "starter" | "hot20";
export type SubmissionStatus =
  | "queued" | "running" | "accepted" | "wrong_answer"
  | "runtime_error" | "time_limit_exceeded" | "internal_error";

export interface Problem {
  id: string;
  title: string;
  description: string;
  language: Language;
  timeLimitMs: number;
  memoryLimitMb: number;
  difficulty: ProblemDifficulty;
  collection: ProblemCollection;
  sortOrder: number;
  tags: string[];
}

export interface JudgeResult {
  status: SubmissionStatus;
  stdout?: string;
  stderr?: string;
  exitCode?: number;
  durationMs?: number;
}

export interface Submission {
  id: string;
  problemId: string;
  language: Language;
  status: SubmissionStatus;
  result?: JudgeResult;
  createdAt: string;
  updatedAt: string;
}

export interface CreateSubmissionInput {
  problemId: string;
  language: Language;
  code: string;
}
