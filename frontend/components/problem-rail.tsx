import Link from "next/link";
import { StatusBadge } from "@/components/status-badge";
import type { Problem, Submission } from "@/lib/types";

interface ProblemRailProps {
  problems: readonly Problem[];
  activeProblemId: string | null;
  recentSubmissions: readonly Submission[];
}

export function ProblemRail({
  problems,
  activeProblemId,
  recentSubmissions,
}: ProblemRailProps) {
  return (
    <aside className="problem-rail" aria-label="Problem browser">
      <nav className="problem-rail__problems" aria-labelledby="problem-list-heading">
        <h2 id="problem-list-heading">Problems</h2>
        {problems.length === 0 ? (
          <p>No problems available.</p>
        ) : (
          <ul>
            {problems.map((problem) => (
              <li key={problem.id}>
                <Link
                  href={`/problems/${encodeURIComponent(problem.id)}`}
                  aria-current={problem.id === activeProblemId ? "page" : undefined}
                >
                  {problem.title}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </nav>

      <section
        className="problem-rail__recent"
        aria-labelledby="recent-submissions-heading"
      >
        <h2 id="recent-submissions-heading">Recent submissions</h2>
        {recentSubmissions.length === 0 ? (
          <p>No recent submissions.</p>
        ) : (
          <ul>
            {recentSubmissions.map((submission) => {
              const problem = problems.find(({ id }) => id === submission.problemId);

              return (
                <li key={submission.id}>
                  <Link href={`/problems/${encodeURIComponent(submission.problemId)}`}>
                    {problem?.title ?? submission.problemId}
                  </Link>
                  <StatusBadge status={submission.status} />
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </aside>
  );
}
