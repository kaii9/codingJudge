import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useSubmissionPolling } from "@/hooks/use-submission-polling";
import type { Submission } from "@/lib/types";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

const queuedSubmission: Submission = {
  id: "sub-1",
  problemId: "sum",
  language: "go",
  status: "queued",
  createdAt: "2026-06-27T00:00:00Z",
  updatedAt: "2026-06-27T00:00:00Z",
};

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

it("polls queued submissions and stops at accepted", async () => {
  const load = vi.fn()
    .mockResolvedValueOnce(queuedSubmission)
    .mockResolvedValueOnce({ ...queuedSubmission, status: "running" })
    .mockResolvedValueOnce({
      ...queuedSubmission,
      status: "accepted",
      result: { status: "accepted" },
    });
  const { result } = renderHook(() => useSubmissionPolling("sub-1", load, 1000));

  await advance(0);
  await advance(1000);
  await advance(1000);

  expect(result.current.submission?.status).toBe("accepted");
  expect(result.current.isPolling).toBe(false);
  expect(load).toHaveBeenCalledTimes(3);
  await vi.advanceTimersByTimeAsync(3000);
  expect(load).toHaveBeenCalledTimes(3);
});

it("aborts the active request on unmount", () => {
  const load = vi.fn((_id: string, signal: AbortSignal) =>
    new Promise<Submission>((_resolve, reject) => {
      signal.addEventListener("abort", () => {
        reject(new DOMException("Aborted", "AbortError"));
      });
    }),
  );
  const { unmount } = renderHook(() => useSubmissionPolling("sub-1", load, 1000));

  unmount();

  expect(load.mock.calls[0][1].aborted).toBe(true);
});

it("returns to idle when the submission id is empty", async () => {
  const load = vi.fn().mockResolvedValue({
    ...queuedSubmission,
    status: "accepted",
    result: { status: "accepted" },
  });
  const { result, rerender } = renderHook(
    ({ id }: { id: string }) => useSubmissionPolling(id, load, 1000),
    { initialProps: { id: "sub-1" } },
  );
  await advance(0);

  rerender({ id: "" });

  expect(result.current.submission).toBeNull();
  expect(result.current.isPolling).toBe(false);
  expect(load).toHaveBeenCalledTimes(1);
});

it("aborts the active request and starts polling the new id", () => {
  const signals: AbortSignal[] = [];
  const load = vi.fn((id: string, signal: AbortSignal) => {
    signals.push(signal);
    return new Promise<Submission>((_resolve, reject) => {
      signal.addEventListener("abort", () => {
        reject(new DOMException(`Aborted ${id}`, "AbortError"));
      });
    });
  });
  const { rerender, unmount } = renderHook(
    ({ id }: { id: string }) => useSubmissionPolling(id, load, 1000),
    { initialProps: { id: "sub-1" } },
  );

  rerender({ id: "sub-2" });

  expect(load).toHaveBeenCalledTimes(2);
  expect(load.mock.calls[1][0]).toBe("sub-2");
  expect(signals[0].aborted).toBe(true);
  expect(signals[1].aborted).toBe(false);
  unmount();
});

it("retries immediately after a polling error", async () => {
  const acceptedSubmission: Submission = {
    ...queuedSubmission,
    status: "accepted",
    result: { status: "accepted" },
  };
  const load = vi.fn()
    .mockRejectedValueOnce(new Error("network unavailable"))
    .mockResolvedValueOnce(acceptedSubmission);
  const { result } = renderHook(() => useSubmissionPolling("sub-1", load, 1000));

  await advance(0);
  expect(result.current.error).toBe("network unavailable");
  expect(result.current.isPolling).toBe(false);

  act(() => result.current.retry());
  await advance(0);

  expect(load).toHaveBeenCalledTimes(2);
  expect(result.current.error).toBeNull();
  expect(result.current.submission).toEqual(acceptedSubmission);
});

it("ignores a stale response after the submission id changes", async () => {
  const first = deferred<Submission>();
  const second = deferred<Submission>();
  const load = vi.fn()
    .mockImplementationOnce(() => first.promise)
    .mockImplementationOnce(() => second.promise);
  const { result, rerender } = renderHook(
    ({ id }: { id: string }) => useSubmissionPolling(id, load, 1000),
    { initialProps: { id: "sub-1" } },
  );
  const latestSubmission: Submission = {
    ...queuedSubmission,
    id: "sub-2",
    status: "accepted",
    result: { status: "accepted" },
  };

  rerender({ id: "sub-2" });
  await act(async () => second.resolve(latestSubmission));
  await act(async () => first.resolve({ ...queuedSubmission, status: "running" }));

  expect(result.current.submission).toEqual(latestSubmission);
  expect(result.current.isPolling).toBe(false);
});

it("waits for each request to finish before scheduling the next one", async () => {
  const requests: Array<ReturnType<typeof deferred<Submission>>> = [];
  let activeRequests = 0;
  let maxActiveRequests = 0;
  const load = vi.fn(() => {
    const request = deferred<Submission>();
    requests.push(request);
    activeRequests += 1;
    maxActiveRequests = Math.max(maxActiveRequests, activeRequests);
    return request.promise.finally(() => {
      activeRequests -= 1;
    });
  });
  const { result } = renderHook(() => useSubmissionPolling("sub-1", load, 1000));

  await advance(5000);
  expect(load).toHaveBeenCalledTimes(1);

  await act(async () => requests[0].resolve(queuedSubmission));
  await advance(999);
  expect(load).toHaveBeenCalledTimes(1);
  await advance(1);
  expect(load).toHaveBeenCalledTimes(2);

  await act(async () => requests[1].resolve({
    ...queuedSubmission,
    status: "accepted",
    result: { status: "accepted" },
  }));
  expect(maxActiveRequests).toBe(1);
  expect(result.current.isPolling).toBe(false);
});
