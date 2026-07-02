import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Home from "@/app/page";
import { ProblemRail } from "@/components/problem-rail";
import { ProblemStatement } from "@/components/problem-statement";
import type { Problem, Submission } from "@/lib/types";

const redirectMock = vi.hoisted(() => vi.fn());
const PROBLEM_FETCH_TIMEOUT_MS = 5_000;

vi.mock("next/navigation", () => ({ redirect: redirectMock }));

const problems: Problem[] = [
  {
    id: "sum",
    title: "A+B",
    description: "Add two numbers",
    language: "go",
    timeLimitMs: 1000,
    memoryLimitMb: 64,
    difficulty: "easy",
    collection: "starter",
    sortOrder: 1,
    tags: ["math"],
  },
  {
    id: "arrays/two words",
    title: "Array Pair",
    description: "Find a matching pair",
    language: "cpp",
    timeLimitMs: 1500,
    memoryLimitMb: 128,
    difficulty: "medium",
    collection: "starter",
    sortOrder: 2,
    tags: ["array", "hash-table"],
  },
  {
    id: "target-pair",
    title: "Target Pair",
    description: "Find two values with a target sum",
    language: "go",
    timeLimitMs: 1000,
    memoryLimitMb: 128,
    difficulty: "easy",
    collection: "hot20",
    sortOrder: 1,
    tags: ["array", "hash-table"],
  },
  {
    id: "course-dependency-order",
    title: "Course Dependency Order",
    description: "Find a valid dependency order",
    language: "go",
    timeLimitMs: 2000,
    memoryLimitMb: 256,
    difficulty: "medium",
    collection: "hot20",
    sortOrder: 2,
    tags: ["graph", "topological-sort"],
  },
];

const recentSubmission: Submission & { sourceCode: string; hiddenInput: string } = {
  id: "sub-1",
  problemId: "sum",
  language: "go",
  status: "accepted",
  createdAt: "2026-06-27T00:00:00Z",
  updatedAt: "2026-06-27T00:00:01Z",
  sourceCode: "PRIVATE SOURCE SENTINEL",
  hiddenInput: "PRIVATE INPUT SENTINEL",
};

beforeEach(() => {
  redirectMock.mockReset();
  vi.stubEnv("API_INTERNAL_URL", "http://api:8080/");
});

afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.restoreAllMocks();
  vi.useRealTimers();
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("ProblemRail", () => {
  it("shows the Hot 20 collection by default", () => {
    render(
      <ProblemRail problems={problems} activeProblemId={null} recentSubmissions={[]} />,
    );

    expect(screen.getByRole("button", { name: "Hot 20" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("link", { name: "Target Pair" })).toBeVisible();
    expect(screen.queryByRole("link", { name: "A+B" })).not.toBeInTheDocument();
  });

  it("links problems, URL-encodes IDs, and marks only the active problem", () => {
    render(
      <ProblemRail
        problems={problems}
        activeProblemId="sum"
        recentSubmissions={[]}
      />,
    );

    const navigation = screen.getByRole("navigation", { name: "Problems" });
    const activeLink = within(navigation).getByRole("link", { name: "A+B" });
    const encodedLink = within(navigation).getByRole("link", { name: "Array Pair" });

    expect(activeLink).toHaveAttribute("href", "/problems/sum");
    expect(activeLink).toHaveAttribute("aria-current", "page");
    expect(encodedLink).toHaveAttribute(
      "href",
      "/problems/arrays%2Ftwo%20words",
    );
    expect(encodedLink).not.toHaveAttribute("aria-current");
    expect(screen.getByRole("button", { name: "Starter" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  it("searches titles and tags case-insensitively", async () => {
    const user = userEvent.setup();
    render(
      <ProblemRail problems={problems} activeProblemId={null} recentSubmissions={[]} />,
    );

    await user.type(screen.getByRole("searchbox", { name: "Search problems" }), "GRAPH");

    expect(screen.getByRole("link", { name: "Course Dependency Order" })).toBeVisible();
    expect(screen.queryByRole("link", { name: "Target Pair" })).not.toBeInTheDocument();
  });

  it("combines collection and difficulty filters", async () => {
    const user = userEvent.setup();
    render(
      <ProblemRail problems={problems} activeProblemId={null} recentSubmissions={[]} />,
    );

    await user.selectOptions(screen.getByRole("combobox", { name: "Difficulty" }), "medium");

    expect(screen.getByRole("link", { name: "Course Dependency Order" })).toBeVisible();
    expect(screen.queryByRole("link", { name: "Target Pair" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Array Pair" })).not.toBeInTheDocument();
  });

  it("renders a stable empty state when no problem matches", async () => {
    const user = userEvent.setup();
    render(
      <ProblemRail problems={problems} activeProblemId={null} recentSubmissions={[]} />,
    );

    await user.type(screen.getByRole("searchbox", { name: "Search problems" }), "missing");

    expect(screen.getByRole("navigation", { name: "Problems" })).toHaveTextContent(
      "No matching problems.",
    );
  });

  it("renders focused recent statuses without exposing private submission data", () => {
    render(
      <ProblemRail
        problems={problems}
        activeProblemId="sum"
        recentSubmissions={[recentSubmission]}
      />,
    );

    const recent = screen.getByRole("region", { name: "Recent submissions" });
    expect(within(recent).getByText("A+B")).toBeVisible();
    expect(within(recent).getByText("Accepted")).toBeVisible();
    expect(screen.queryByText("PRIVATE SOURCE SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE INPUT SENTINEL")).not.toBeInTheDocument();
  });

  it("keeps empty problem and recent lists accessible and stable", () => {
    render(
      <ProblemRail problems={[]} activeProblemId={null} recentSubmissions={[]} />,
    );

    expect(screen.getByRole("navigation", { name: "Problems" })).toHaveTextContent(
      "No problems available.",
    );
    expect(screen.getByRole("region", { name: "Recent submissions" })).toHaveTextContent(
      "No recent submissions.",
    );
  });
});

describe("ProblemStatement", () => {
  it("renders every public problem field and no private judge data", () => {
    const problemWithPrivateData = {
      ...problems[0],
      description: "Read two integers.\nPrint their sum.",
      testCases: ["PRIVATE TEST CASE SENTINEL"],
      expectedOutput: "PRIVATE OUTPUT SENTINEL",
      hiddenInput: "PRIVATE INPUT SENTINEL",
      sourceCode: "PRIVATE SOURCE SENTINEL",
    };

    render(<ProblemStatement problem={problemWithPrivateData} />);

    expect(screen.getByRole("heading", { level: 1, name: "A+B" })).toBeVisible();
    expect(
      screen.getByRole("region", { name: "Problem statement" }).querySelector("p"),
    ).toHaveTextContent("Read two integers.Print their sum.");
    expect(screen.getByText("1000 ms")).toBeVisible();
    expect(screen.getByText("64 MB")).toBeVisible();
    expect(screen.getByText("go")).toBeVisible();
    expect(screen.getByText("Easy")).toBeVisible();
    expect(screen.getByText("math")).toBeVisible();
    expect(screen.queryByText("PRIVATE TEST CASE SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE OUTPUT SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE INPUT SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE SOURCE SENTINEL")).not.toBeInTheDocument();
  });

  it("preserves CRLF and LF description line breaks without parsing markup", () => {
    render(
      <ProblemStatement
        problem={{
          ...problems[0],
          description:
            "Read two integers.\r\n<strong>Keep this literal.</strong>\nPrint their sum.",
        }}
      />,
    );

    const statement = screen.getByRole("region", { name: "Problem statement" });
    const description = statement.querySelector("p");

    expect(description).not.toBeNull();
    expect(description?.querySelectorAll("br")).toHaveLength(2);
    expect(description).toHaveTextContent("<strong>Keep this literal.</strong>");
    expect(description?.querySelector("strong")).toBeNull();
  });
});

describe("root problem selection", () => {
  it("fetches on the server without caching and redirects to the encoded first ID", async () => {
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        Response.json([{ ...problems[0], id: "arrays/two words" }]),
      ),
    );
    redirectMock.mockImplementation((path: string) => {
      throw new Error(`redirect:${path}`);
    });

    await expect(Home()).rejects.toThrow(
      "redirect:/problems/arrays%2Ftwo%20words",
    );
    expect(fetch).toHaveBeenCalledWith(
      "http://api:8080/problems",
      expect.objectContaining({
        cache: "no-store",
        signal: expect.any(AbortSignal),
      }),
    );
    expect(redirectMock).toHaveBeenCalledWith(
      "/problems/arrays%2Ftwo%20words",
    );

    const deadlineCallIndex = setTimeoutSpy.mock.calls.findIndex(
      ([, delay]) => delay === PROBLEM_FETCH_TIMEOUT_MS,
    );
    expect(deadlineCallIndex).toBeGreaterThanOrEqual(0);
    expect(clearTimeoutSpy).toHaveBeenCalledWith(
      setTimeoutSpy.mock.results[deadlineCallIndex]?.value,
    );
  });

  it("uses the local backend default and renders a stable valid-empty state", async () => {
    vi.stubEnv("API_INTERNAL_URL", "");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json([])));

    render(await Home());

    expect(fetch).toHaveBeenCalledWith(
      "http://localhost:8080/problems",
      expect.objectContaining({
        cache: "no-store",
        signal: expect.any(AbortSignal),
      }),
    );
    expect(screen.getByRole("heading", { name: "No problems available" })).toBeVisible();
    expect(screen.getByText("There are no problems to solve yet.")).toBeVisible();
    expect(redirectMock).not.toHaveBeenCalled();
  });

  it("aborts a stalled backend request at the deadline and renders the safe error state", async () => {
    vi.useFakeTimers();
    let requestSignal: AbortSignal | undefined;
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");

    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((_input: RequestInfo | URL, init?: RequestInit) => {
        requestSignal = init?.signal ?? undefined;

        return new Promise<Response>((_resolve, reject) => {
          requestSignal?.addEventListener("abort", () => {
            reject(new DOMException("SECRET timeout detail", "AbortError"));
          });
        });
      }),
    );

    const page = Home();
    const deadlineCallIndex = setTimeoutSpy.mock.calls.findIndex(
      ([, delay]) => delay === PROBLEM_FETCH_TIMEOUT_MS,
    );

    expect(deadlineCallIndex).toBeGreaterThanOrEqual(0);
    expect(requestSignal).toBeInstanceOf(AbortSignal);

    await vi.advanceTimersByTimeAsync(PROBLEM_FETCH_TIMEOUT_MS);
    render(await page);

    expect(requestSignal?.aborted).toBe(true);
    expect(clearTimeoutSpy).toHaveBeenCalledWith(
      setTimeoutSpy.mock.results[deadlineCallIndex]?.value,
    );
    expect(
      screen.getByRole("heading", { name: "Problems unavailable" }),
    ).toBeVisible();
    expect(screen.queryByText(/SECRET|api:8080/i)).not.toBeInTheDocument();
  });

  it("clears the backend deadline when fetch rejects before timeout", async () => {
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockRejectedValue(new Error("SECRET early failure")),
    );

    render(await Home());

    const deadlineCallIndex = setTimeoutSpy.mock.calls.findIndex(
      ([, delay]) => delay === PROBLEM_FETCH_TIMEOUT_MS,
    );
    expect(deadlineCallIndex).toBeGreaterThanOrEqual(0);
    expect(clearTimeoutSpy).toHaveBeenCalledWith(
      setTimeoutSpy.mock.results[deadlineCallIndex]?.value,
    );
    expect(
      screen.getByRole("heading", { name: "Problems unavailable" }),
    ).toBeVisible();
    expect(screen.queryByText(/SECRET|api:8080/i)).not.toBeInTheDocument();
  });

  it.each([
    ["network failure", () => Promise.reject(new Error("SECRET internal host"))],
    ["non-2xx response", () => Promise.resolve(Response.json([], { status: 503 }))],
    [
      "malformed JSON",
      () =>
        Promise.resolve(
          new Response("not-json", {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
        ),
    ],
    ["non-array JSON", () => Promise.resolve(Response.json({ problems: [] }))],
    [
      "wrong-shaped problem",
      () => Promise.resolve(Response.json([{ id: "sum", title: "A+B" }])),
    ],
  ])("renders one safe backend-unavailable state for %s", async (_case, response) => {
    vi.stubGlobal("fetch", vi.fn(response));

    render(await Home());

    expect(
      screen.getByRole("heading", { name: "Problems unavailable" }),
    ).toBeVisible();
    expect(screen.getByText("The problem service is unavailable. Try again later.")).toBeVisible();
    expect(screen.queryByText(/SECRET|api:8080/i)).not.toBeInTheDocument();
    expect(redirectMock).not.toHaveBeenCalled();
  });
});
