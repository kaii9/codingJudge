import { act, cleanup, render, screen } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppShell } from "@/components/app-shell";

const HEALTH_CHECK_TIMEOUT_MS = 5_000;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });

  return { promise, reject, resolve };
}

afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.restoreAllMocks();
  vi.useRealTimers();
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

  it("ignores a healthy body on a non-2xx response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response('{"status":"ok"}', {
          status: 503,
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

  it("reports malformed health JSON as unavailable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response("not-json", {
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

  it("times out a stalled health request and aborts it", async () => {
    vi.useFakeTimers();
    let requestSignal: AbortSignal | undefined;
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");

    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation((_input: RequestInfo | URL, init?: RequestInit) => {
        requestSignal = init?.signal ?? undefined;
        return new Promise<Response>(() => undefined);
      }),
    );

    render(
      <AppShell>
        <main>Cached content</main>
      </AppShell>,
    );

    expect(screen.getByText("Checking")).toBeVisible();
    expect(setTimeoutSpy).toHaveBeenCalledWith(
      expect.any(Function),
      HEALTH_CHECK_TIMEOUT_MS,
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(HEALTH_CHECK_TIMEOUT_MS);
    });

    expect(screen.getByText("Unavailable")).toBeVisible();
    expect(requestSignal?.aborted).toBe(true);
  });

  it("clears the health deadline after normal completion", async () => {
    vi.useFakeTimers();
    const response = deferred<Response>();
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");
    vi.stubGlobal("fetch", vi.fn().mockReturnValue(response.promise));

    render(
      <AppShell>
        <main>Content</main>
      </AppShell>,
    );

    const deadlineCallIndex = setTimeoutSpy.mock.calls.findIndex(
      ([, delay]) => delay === HEALTH_CHECK_TIMEOUT_MS,
    );
    expect(deadlineCallIndex).toBeGreaterThanOrEqual(0);
    const deadlineId = setTimeoutSpy.mock.results[deadlineCallIndex]?.value;

    await act(async () => {
      response.resolve(
        new Response('{"status":"ok"}', {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
      await response.promise;
    });

    expect(screen.getByText("Online")).toBeVisible();
    expect(clearTimeoutSpy).toHaveBeenCalledWith(deadlineId);
  });

  it("keeps the current StrictMode result when an aborted request resolves late", async () => {
    const firstResponse = deferred<Response>();
    const secondResponse = deferred<Response>();
    const requestSignals: AbortSignal[] = [];
    const fetchMock = vi
      .fn()
      .mockImplementationOnce((_input: RequestInfo | URL, init?: RequestInit) => {
        requestSignals.push(init?.signal as AbortSignal);
        return firstResponse.promise;
      })
      .mockImplementationOnce((_input: RequestInfo | URL, init?: RequestInit) => {
        requestSignals.push(init?.signal as AbortSignal);
        return secondResponse.promise;
      });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <StrictMode>
        <AppShell>
          <main>Content</main>
        </AppShell>
      </StrictMode>,
    );

    await act(async () => {
      secondResponse.resolve(
        new Response('{"status":"ok"}', {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
      await secondResponse.promise;
    });

    expect(screen.getByText("Online")).toBeVisible();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(requestSignals[0]?.aborted).toBe(true);
    expect(requestSignals[1]?.aborted).toBe(false);

    await act(async () => {
      firstResponse.resolve(
        new Response('{"status":"degraded"}', {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
      await firstResponse.promise;
    });

    expect(screen.getByText("Online")).toBeVisible();
    expect(screen.queryByText("Unavailable")).not.toBeInTheDocument();
  });

  it("aborts the health request on unmount", async () => {
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

    const deadlineCallIndex = setTimeoutSpy.mock.calls.findIndex(
      ([, delay]) => delay === HEALTH_CHECK_TIMEOUT_MS,
    );
    expect(deadlineCallIndex).toBeGreaterThanOrEqual(0);
    const deadlineId = setTimeoutSpy.mock.results[deadlineCallIndex]?.value;

    await act(async () => {
      unmount();
      await Promise.resolve();
    });

    expect(requestSignal).toBeInstanceOf(AbortSignal);
    expect(requestSignal?.aborted).toBe(true);
    expect(clearTimeoutSpy).toHaveBeenCalledWith(deadlineId);
  });
});
