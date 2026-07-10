"use client";

import { RotateCcw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { getLeaderboard } from "@/lib/api";
import type { LeaderboardEntry } from "@/lib/types";

type LoadState =
  | { kind: "loading" }
  | { kind: "error" }
  | { kind: "ready"; entries: LeaderboardEntry[] };

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat("en", {
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export default function LeaderboardPage() {
  const [state, setState] = useState<LoadState>({ kind: "loading" });

  const retry = useCallback(() => {
    setState({ kind: "loading" });
    void getLeaderboard().then(
      entries => setState({ kind: "ready", entries }),
      () => setState({ kind: "error" }),
    );
  }, []);

  useEffect(() => {
    let active = true;

    void getLeaderboard().then(
      entries => {
        if (active) setState({ kind: "ready", entries });
      },
      () => {
        if (active) setState({ kind: "error" });
      },
    );

    return () => {
      active = false;
    };
  }, []);

  if (state.kind === "loading") {
    return (
      <main className="leaderboard-page" aria-busy="true">
        <h1>Leaderboard</h1>
        <p>Loading accepted submissions.</p>
      </main>
    );
  }

  if (state.kind === "error") {
    return (
      <main className="leaderboard-page">
        <h1>Leaderboard</h1>
        <p>Unable to load the leaderboard.</p>
        <button type="button" onClick={retry}>
          <RotateCcw size={16} aria-hidden="true" />
          <span>Retry</span>
        </button>
      </main>
    );
  }

  return (
    <main className="leaderboard-page">
      <div className="leaderboard-page__header">
        <h1>Leaderboard</h1>
        <p>Ranked by solved problems, then earliest latest accepted time.</p>
      </div>
      <div className="leaderboard-table" role="region" aria-label="Leaderboard table">
        <table>
          <thead>
            <tr>
              <th scope="col">Rank</th>
              <th scope="col">User</th>
              <th scope="col">Solved</th>
              <th scope="col">Accepted</th>
              <th scope="col">Last AC</th>
            </tr>
          </thead>
          <tbody>
            {state.entries.length === 0 ? (
              <tr>
                <td colSpan={5}>No accepted submissions yet.</td>
              </tr>
            ) : state.entries.map(entry => (
              <tr key={entry.userId}>
                <td>{entry.rank}</td>
                <td>{entry.username}</td>
                <td>{entry.solved}</td>
                <td>{entry.acceptedSubmissions}</td>
                <td>{formatTime(entry.lastAcceptedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </main>
  );
}
