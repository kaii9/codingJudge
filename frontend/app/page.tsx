import { redirect } from "next/navigation";
import type { Language, Problem } from "@/lib/types";

const DEFAULT_API_INTERNAL_URL = "http://localhost:8080";
const supportedLanguages = new Set<Language>(["go", "cpp", "python"]);

function isProblem(value: unknown): value is Problem {
  if (typeof value !== "object" || value === null) return false;

  const candidate = value as Partial<Problem>;
  return (
    typeof candidate.id === "string" &&
    typeof candidate.title === "string" &&
    typeof candidate.description === "string" &&
    typeof candidate.language === "string" &&
    supportedLanguages.has(candidate.language as Language) &&
    typeof candidate.timeLimitMs === "number" &&
    Number.isFinite(candidate.timeLimitMs) &&
    typeof candidate.memoryLimitMb === "number" &&
    Number.isFinite(candidate.memoryLimitMb)
  );
}

async function loadProblems(): Promise<Problem[] | null> {
  const baseUrl = (process.env.API_INTERNAL_URL?.trim() || DEFAULT_API_INTERNAL_URL)
    .replace(/\/+$/, "");

  try {
    const response = await fetch(`${baseUrl}/problems`, { cache: "no-store" });
    if (!response.ok) return null;

    const payload: unknown = await response.json();
    return Array.isArray(payload) && payload.every(isProblem) ? payload : null;
  } catch {
    return null;
  }
}

function ProblemIndexState({
  title,
  message,
}: {
  title: string;
  message: string;
}) {
  return (
    <main className="problem-index-state">
      <h1>{title}</h1>
      <p>{message}</p>
    </main>
  );
}

export default async function Home() {
  const problems = await loadProblems();

  if (problems === null) {
    return (
      <ProblemIndexState
        title="Problems unavailable"
        message="The problem service is unavailable. Try again later."
      />
    );
  }

  if (problems.length === 0) {
    return (
      <ProblemIndexState
        title="No problems available"
        message="There are no problems to solve yet."
      />
    );
  }

  redirect(`/problems/${encodeURIComponent(problems[0].id)}`);
}
