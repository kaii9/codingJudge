"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { StatusBadge } from "@/components/status-badge";
import type {
  Problem,
  ProblemCollection,
  ProblemDifficulty,
  Submission,
} from "@/lib/types";

interface ProblemRailProps {
  problems: readonly Problem[];
  activeProblemId: string | null;
  recentSubmissions: readonly Submission[];
}

type CollectionFilter = ProblemCollection | "all";
type DifficultyFilter = ProblemDifficulty | "all";

const collectionOptions: ReadonlyArray<{
  value: CollectionFilter;
  label: string;
}> = [
  { value: "hot20", label: "Hot 20" },
  { value: "starter", label: "Starter" },
  { value: "all", label: "All" },
];

function collectionForProblem(
  problems: readonly Problem[],
  activeProblemId: string | null,
): CollectionFilter {
  return problems.find(({ id }) => id === activeProblemId)?.collection ?? "hot20";
}

function difficultyLabel(difficulty: ProblemDifficulty) {
  return difficulty[0].toUpperCase() + difficulty.slice(1);
}

export function ProblemRail({
  problems,
  activeProblemId,
  recentSubmissions,
}: ProblemRailProps) {
  const [collectionSelection, setCollectionSelection] = useState<{
    activeProblemId: string | null;
    value: CollectionFilter;
  }>(() => ({
    activeProblemId,
    value: collectionForProblem(problems, activeProblemId),
  }));
  const [difficulty, setDifficulty] = useState<DifficultyFilter>("all");
  const [query, setQuery] = useState("");
  const collection =
    collectionSelection.activeProblemId === activeProblemId
      ? collectionSelection.value
      : collectionForProblem(problems, activeProblemId);

  const filteredProblems = useMemo(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();

    return problems.filter((problem) => {
      const matchesCollection =
        collection === "all" || problem.collection === collection;
      const matchesDifficulty =
        difficulty === "all" || problem.difficulty === difficulty;
      const matchesQuery =
        normalizedQuery.length === 0 ||
        problem.title.toLocaleLowerCase().includes(normalizedQuery) ||
        problem.tags.some((tag) =>
          tag.toLocaleLowerCase().includes(normalizedQuery),
        );

      return matchesCollection && matchesDifficulty && matchesQuery;
    });
  }, [collection, difficulty, problems, query]);

  return (
    <aside className="problem-rail" aria-label="Problem browser">
      <nav className="problem-rail__problems" aria-labelledby="problem-list-heading">
        <h2 id="problem-list-heading">Problems</h2>
        {problems.length === 0 ? (
          <p>No problems available.</p>
        ) : (
          <>
            <div className="problem-rail__filters">
              <div className="problem-rail__collections" aria-label="Problem collection">
                {collectionOptions.map((option) => (
                  <button
                    key={option.value}
                    type="button"
                    aria-pressed={collection === option.value}
                    onClick={() =>
                      setCollectionSelection({
                        activeProblemId,
                        value: option.value,
                      })
                    }
                  >
                    {option.label}
                  </button>
                ))}
              </div>
              <input
                type="search"
                aria-label="Search problems"
                placeholder="Search title or tag"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
              />
              <select
                aria-label="Difficulty"
                value={difficulty}
                onChange={(event) =>
                  setDifficulty(event.target.value as DifficultyFilter)
                }
              >
                <option value="all">All difficulties</option>
                <option value="easy">Easy</option>
                <option value="medium">Medium</option>
                <option value="hard">Hard</option>
              </select>
            </div>
            {filteredProblems.length === 0 ? (
              <p className="problem-rail__empty">No matching problems.</p>
            ) : (
              <ul className="problem-rail__list">
                {filteredProblems.map((problem) => (
                  <li key={problem.id}>
                    <Link
                      href={`/problems/${encodeURIComponent(problem.id)}`}
                      aria-label={problem.title}
                      aria-current={problem.id === activeProblemId ? "page" : undefined}
                    >
                      <span className="problem-rail__title">{problem.title}</span>
                      <span className="problem-rail__metadata" aria-hidden="true">
                        <span data-difficulty={problem.difficulty}>
                          {difficultyLabel(problem.difficulty)}
                        </span>
                        {problem.tags.map((tag) => (
                          <span key={tag}>{tag}</span>
                        ))}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </>
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
