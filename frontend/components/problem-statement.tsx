import { Fragment } from "react";
import type { Problem } from "@/lib/types";

interface ProblemStatementProps {
  problem: Problem;
}

export function ProblemStatement({ problem }: ProblemStatementProps) {
  const difficulty =
    problem.difficulty[0].toUpperCase() + problem.difficulty.slice(1);

  return (
    <article className="problem-statement" aria-labelledby="problem-title">
      <header>
        <h1 id="problem-title">{problem.title}</h1>
        <div className="problem-statement__metadata">
          <span data-difficulty={problem.difficulty}>{difficulty}</span>
          {problem.tags.map((tag) => (
            <span key={tag}>{tag}</span>
          ))}
        </div>
      </header>

      <section aria-labelledby="problem-limits-heading">
        <h2 id="problem-limits-heading">Limits</h2>
        <dl>
          <div>
            <dt>Time limit</dt>
            <dd>{problem.timeLimitMs} ms</dd>
          </div>
          <div>
            <dt>Memory limit</dt>
            <dd>{problem.memoryLimitMb} MB</dd>
          </div>
          <div>
            <dt>Language</dt>
            <dd>{problem.language}</dd>
          </div>
        </dl>
      </section>

      <section aria-labelledby="problem-description-heading">
        <h2 id="problem-description-heading">Problem statement</h2>
        <p>
          {problem.description.split(/\r\n?|\n/).map((line, index) => (
            <Fragment key={index}>
              {index > 0 ? <br /> : null}
              {line}
            </Fragment>
          ))}
        </p>
      </section>
    </article>
  );
}
