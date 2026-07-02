import { Fragment } from "react";
import type { Problem } from "@/lib/types";

interface ProblemStatementProps {
  problem: Problem;
}

export function ProblemStatement({ problem }: ProblemStatementProps) {
  return (
    <article className="problem-statement" aria-labelledby="problem-title">
      <header>
        <h1 id="problem-title">{problem.title}</h1>
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
