import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "@/components/app-shell";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AppShell", () => {
  it("reports healthy service state without blocking navigation or content", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('{"status":"ok"}', {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    render(
      <AppShell>
        <main>Workbench content</main>
      </AppShell>,
    );

    expect(screen.getByRole("link", { name: "Problems" })).toHaveAttribute("href", "/");
    expect(screen.getByRole("link", { name: "Submissions" })).toHaveAttribute(
      "href",
      "/submissions",
    );
    expect(screen.getAllByRole("main")).toHaveLength(1);
    expect(screen.getByRole("main")).toHaveTextContent("Workbench content");
    expect(await screen.findByText("Online")).toBeVisible();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/healthz",
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    );
  });

  it("reports an unavailable service when the health request fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network down")));

    render(
      <AppShell>
        <main>Cached content</main>
      </AppShell>,
    );

    expect(screen.getByRole("main")).toHaveTextContent("Cached content");
    expect(await screen.findByText("Unavailable")).toBeVisible();
  });

  it("treats a successful but unhealthy response as unavailable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response('{"status":"degraded"}', {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      ),
    );

    render(
      <AppShell>
        <main>Cached content</main>
      </AppShell>,
    );

    expect(await screen.findByText("Unavailable")).toBeVisible();
  });

  it("aborts the health request on unmount", async () => {
    let requestSignal: AbortSignal | undefined;
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((_input: RequestInfo | URL, init?: RequestInit) => {
        requestSignal = init?.signal ?? undefined;

        return new Promise<Response>((_resolve, reject) => {
          requestSignal?.addEventListener("abort", () => {
            reject(new DOMException("The operation was aborted", "AbortError"));
          });
        });
      }),
    );

    const { unmount } = render(
      <AppShell>
        <main>Content</main>
      </AppShell>,
    );

    await act(async () => {
      unmount();
      await Promise.resolve();
    });

    expect(requestSignal).toBeInstanceOf(AbortSignal);
    expect(requestSignal?.aborted).toBe(true);
    expect(consoleError).not.toHaveBeenCalled();
  });
});
