import { afterEach, describe, expect, it, vi } from "vitest";
import { backendURL, GET, POST } from "@/app/api/[...path]/route";

const MAX_PROXY_BODY_BYTES = 66_560;

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("backendURL", () => {
  it("preserves path and query without exposing the internal URL", () => {
    expect(backendURL(["submissions", "sub-1"], "?verbose=1", "http://api:8080"))
      .toBe("http://api:8080/submissions/sub-1?verbose=1");
  });
});

describe("API proxy", () => {
  it("forwards GET requests after resolving async params and preserves the response", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://api:8080/");
    const responseBytes = new Uint8Array([0, 1, 2, 255]);
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(new Response(responseBytes, {
      status: 206,
      headers: { "Content-Type": "application/octet-stream" },
    }));
    vi.stubGlobal("fetch", fetchMock);
    const request = new Request("http://localhost/api/problems?verbose=1", {
      headers: { "X-Internal-Header": "do-not-forward" },
    });

    const response = await GET(request, {
      params: Promise.resolve({ path: ["problems"] }),
    });

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://api:8080/problems?verbose=1");
    expect(init?.method).toBe("GET");
    expect(init?.body).toBeUndefined();
    expect(init?.signal).toBe(request.signal);
    expect(Array.from(new Headers(init?.headers).entries())).toEqual([]);
    expect(response.status).toBe(206);
    expect(response.headers.get("content-type")).toBe("application/octet-stream");
    expect(Array.from(new Uint8Array(await response.arrayBuffer())))
      .toEqual(Array.from(responseBytes));
  });

  it("forwards POST body and content type without forwarding other headers", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://api:8080");
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(Response.json(
      { id: "sub-1" },
      { status: 202 },
    ));
    vi.stubGlobal("fetch", fetchMock);
    const body = JSON.stringify({ problemId: "sum", language: "go", code: "package main" });
    const request = new Request("http://localhost/api/submissions", {
      method: "POST",
      headers: {
        "Authorization": "Bearer secret",
        "Content-Type": "application/json",
        "X-Internal-Header": "do-not-forward",
      },
      body,
    });

    const response = await POST(request, {
      params: Promise.resolve({ path: ["submissions"] }),
    });

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://api:8080/submissions");
    expect(init?.method).toBe("POST");
    expect(init?.signal).toBe(request.signal);
    expect(Array.from(new Headers(init?.headers).entries()))
      .toEqual([["content-type", "application/json"]]);
    expect(await new Response(init?.body).text()).toBe(body);
    expect(response.status).toBe(202);
    expect(response.headers.get("content-type")).toBe("application/json");
    await expect(response.json()).resolves.toEqual({ id: "sub-1" });
  });

  it("returns structured JSON when API_INTERNAL_URL is missing", async () => {
    vi.stubEnv("API_INTERNAL_URL", "");
    const fetchMock = vi.fn<typeof fetch>();
    vi.stubGlobal("fetch", fetchMock);

    const response = await GET(
      new Request("http://localhost/api/problems"),
      { params: Promise.resolve({ path: ["problems"] }) },
    );

    expect(fetchMock).not.toHaveBeenCalled();
    expect(response.status).toBe(500);
    await expect(response.json()).resolves.toEqual({
      error: {
        code: "proxy_configuration_error",
        message: "API_INTERNAL_URL is not configured",
      },
    });
  });

  it("normalizes upstream transport failures without leaking the backend URL", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://api.internal:8080");
    vi.stubGlobal("fetch", vi.fn<typeof fetch>().mockRejectedValue(
      new TypeError("fetch failed for http://api.internal:8080/problems"),
    ));

    const response = await GET(
      new Request("http://localhost/api/problems"),
      { params: Promise.resolve({ path: ["problems"] }) },
    );
    const payload = await response.json();

    expect(response.status).toBe(502);
    expect(payload).toEqual({
      error: { code: "backend_unavailable", message: "backend unavailable" },
    });
    expect(JSON.stringify(payload)).not.toContain("api.internal");
  });

  it("propagates and distinguishes client cancellation", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://api:8080");
    const controller = new AbortController();
    const request = new Request("http://localhost/api/problems", {
      signal: controller.signal,
    });
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async (_url, init) => {
      expect(init?.signal).toBe(request.signal);
      controller.abort();
      throw new DOMException("The operation was aborted", "AbortError");
    });
    vi.stubGlobal("fetch", fetchMock);

    const response = await GET(request, {
      params: Promise.resolve({ path: ["problems"] }),
    });

    expect(response.status).toBe(499);
    await expect(response.json()).resolves.toEqual({
      error: { code: "request_cancelled", message: "request was cancelled" },
    });
  });

  it("rejects an oversized Content-Length before reading or forwarding", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://api:8080");
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(Response.json({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    const request = new Request("http://localhost/api/submissions", {
      method: "POST",
      headers: {
        "Content-Length": String(MAX_PROXY_BODY_BYTES + 1),
        "Content-Type": "application/json",
      },
      body: "{}",
    });

    const response = await POST(request, {
      params: Promise.resolve({ path: ["submissions"] }),
    });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(response.status).toBe(413);
    await expect(response.json()).resolves.toEqual({
      error: { code: "request_too_large", message: "request body is too large" },
    });
  });

  it("rejects an oversized chunked body without forwarding it", async () => {
    vi.stubEnv("API_INTERNAL_URL", "http://api:8080");
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(Response.json({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    const chunks = [new Uint8Array(40_000), new Uint8Array(30_000)];
    const body = new ReadableStream<Uint8Array>({
      pull(controller) {
        const chunk = chunks.shift();
        if (chunk) {
          controller.enqueue(chunk);
        } else {
          controller.close();
        }
      },
    });
    const request = new Request("http://localhost/api/submissions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
      duplex: "half",
    } as RequestInit & { duplex: "half" });

    const response = await POST(request, {
      params: Promise.resolve({ path: ["submissions"] }),
    });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(response.status).toBe(413);
    await expect(response.json()).resolves.toEqual({
      error: { code: "request_too_large", message: "request body is too large" },
    });
  });
});
