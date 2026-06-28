import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Home from "@/app/page";
import { ProblemRail } from "@/components/problem-rail";
import { ProblemStatement } from "@/components/problem-statement";
import type { Problem, Submission } from "@/lib/types";

const redirectMock = vi.hoisted(() => vi.fn());

vi.mock("next/navigation", () => ({ redirect: redirectMock }));

const problems: Problem[] = [
  {
    id: "sum",
    title: "A+B",
    description: "Add two numbers",
    language: "go",
    timeLimitMs: 1000,
    memoryLimitMb: 64,
  },
  {
    id: "arrays/two words",
    title: "Array Pair",
    description: "Find a matching pair",
    language: "cpp",
    timeLimitMs: 1500,
    memoryLimitMb: 128,
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
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("ProblemRail", () => {
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
    expect(screen.getByText(/Read two integers\.\s+Print their sum\./)).toBeVisible();
    expect(screen.getByText("1000 ms")).toBeVisible();
    expect(screen.getByText("64 MB")).toBeVisible();
    expect(screen.getByText("go")).toBeVisible();
    expect(screen.queryByText("PRIVATE TEST CASE SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE OUTPUT SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE INPUT SENTINEL")).not.toBeInTheDocument();
    expect(screen.queryByText("PRIVATE SOURCE SENTINEL")).not.toBeInTheDocument();
  });
});

describe("root problem selection", () => {
  it("fetches on the server without caching and redirects to the encoded first ID", async () => {
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
    expect(fetch).toHaveBeenCalledWith("http://api:8080/problems", {
      cache: "no-store",
    });
    expect(redirectMock).toHaveBeenCalledWith(
      "/problems/arrays%2Ftwo%20words",
    );
  });

  it("uses the local backend default and renders a stable valid-empty state", async () => {
    vi.stubEnv("API_INTERNAL_URL", "");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json([])));

    render(await Home());

    expect(fetch).toHaveBeenCalledWith("http://localhost:8080/problems", {
      cache: "no-store",
    });
    expect(screen.getByRole("heading", { name: "No problems available" })).toBeVisible();
    expect(screen.getByText("There are no problems to solve yet.")).toBeVisible();
    expect(redirectMock).not.toHaveBeenCalled();
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
