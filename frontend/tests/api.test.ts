import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, createSubmission, getLeaderboard, getProblems } from "@/lib/api";

afterEach(() => vi.unstubAllGlobals());

describe("getProblems", () => {
  it("returns typed problems", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify([
      { id: "sum", title: "A+B", description: "Add", language: "go", timeLimitMs: 1000, memoryLimitMb: 64 },
    ]), { status: 200 })));

    await expect(getProblems()).resolves.toMatchObject([{ id: "sum", title: "A+B" }]);
  });

  it("normalizes structured API errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "backend_unavailable", message: "service unavailable" },
    }), { status: 503 })));

    await expect(getProblems()).rejects.toEqual(
      new ApiError(503, "backend_unavailable", "service unavailable"),
    );
  });

  it("rejects malformed successful responses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not json", {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })));

    await expect(getProblems()).rejects.toEqual(
      new ApiError(200, "invalid_response", "API returned an invalid JSON response"),
    );
  });
});

describe("getLeaderboard", () => {
  it("returns leaderboard entries", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json([
      {
        rank: 1,
        userId: "user-1",
        username: "kai",
        solved: 3,
        acceptedSubmissions: 4,
        lastAcceptedAt: "2026-07-11T00:00:00Z",
      },
    ])));

    await expect(getLeaderboard()).resolves.toMatchObject([
      { rank: 1, username: "kai", solved: 3, acceptedSubmissions: 4 },
    ]);
    expect(fetch).toHaveBeenCalledWith("/api/leaderboard", undefined);
  });
});

describe("createSubmission", () => {
  it("adds a unique idempotency key", async () => {
    const fetchMock = vi.fn().mockResolvedValue(Response.json({ id: "sub-1" }, { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);

    await createSubmission({ problemId: "sum", language: "go", code: "package main" });

    const [, init] = fetchMock.mock.calls[0];
    const key = new Headers(init?.headers).get("idempotency-key");
    expect(key).toMatch(/^[0-9a-f-]{36}$/);
  });
});
