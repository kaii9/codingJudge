import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, getProblems } from "@/lib/api";

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
});
