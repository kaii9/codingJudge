type ProxyContext = {
  params: Promise<{ path: string[] }>;
};

const MAX_REQUEST_BODY_BYTES = 66_560;

class RequestTooLargeError extends Error {}

function errorResponse(status: number, code: string, message: string): Response {
  return Response.json({ error: { code, message } }, { status });
}

async function readWithSignal(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  signal: AbortSignal,
): Promise<ReadableStreamReadResult<Uint8Array>> {
  if (signal.aborted) {
    throw signal.reason;
  }

  return new Promise((resolve, reject) => {
    const onAbort = () => reject(signal.reason);
    signal.addEventListener("abort", onAbort, { once: true });
    reader.read().then(
      (result) => {
        signal.removeEventListener("abort", onAbort);
        resolve(result);
      },
      (error: unknown) => {
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

async function readRequestBody(request: Request): Promise<ArrayBuffer | undefined> {
  if (request.method === "GET" || request.method === "HEAD" || !request.body) {
    return undefined;
  }

  const contentLength = Number(request.headers.get("content-length"));
  if (Number.isFinite(contentLength) && contentLength > MAX_REQUEST_BODY_BYTES) {
    throw new RequestTooLargeError();
  }

  const reader = request.body.getReader();
  const chunks: Uint8Array[] = [];
  let totalBytes = 0;

  try {
    while (true) {
      const { done, value } = await readWithSignal(reader, request.signal);
      if (done) break;
      if (totalBytes + value.byteLength > MAX_REQUEST_BODY_BYTES) {
        try {
          await reader.cancel();
        } catch {
          // The size error remains the actionable failure.
        }
        throw new RequestTooLargeError();
      }
      chunks.push(value);
      totalBytes += value.byteLength;
    }
  } catch (error) {
    if (request.signal.aborted) {
      try {
        await reader.cancel(request.signal.reason);
      } catch {
        // The abort remains the actionable failure.
      }
    }
    throw error;
  } finally {
    reader.releaseLock();
  }

  const body = new Uint8Array(totalBytes);
  let offset = 0;
  for (const chunk of chunks) {
    body.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return body.buffer;
}

export function backendURL(path: string[], search: string, base = process.env.API_INTERNAL_URL ?? "") {
  if (!base) throw new Error("API_INTERNAL_URL is not configured");
  return `${base.replace(/\/$/, "")}/${path.map(encodeURIComponent).join("/")}${search}`;
}

export async function proxy(request: Request, context: ProxyContext): Promise<Response> {
  if (!process.env.API_INTERNAL_URL) {
    return errorResponse(
      500,
      "proxy_configuration_error",
      "API_INTERNAL_URL is not configured",
    );
  }

  const { path } = await context.params;
  const contentType = request.headers.get("content-type");
  const headers = new Headers();
  if (contentType) headers.set("content-type", contentType);

  const method = request.method.toUpperCase();
  let body: ArrayBuffer | undefined;
  try {
    body = await readRequestBody(request);
  } catch (error) {
    if (error instanceof RequestTooLargeError) {
      return errorResponse(413, "request_too_large", "request body is too large");
    }
    if (request.signal.aborted) {
      return errorResponse(499, "request_cancelled", "request was cancelled");
    }
    return errorResponse(400, "invalid_request", "failed to read request body");
  }

  let response: Response;
  try {
    response = await fetch(
      backendURL(path, new URL(request.url).search, process.env.API_INTERNAL_URL),
      { method, headers, body, signal: request.signal },
    );
  } catch {
    if (request.signal.aborted) {
      return errorResponse(499, "request_cancelled", "request was cancelled");
    }
    return errorResponse(502, "backend_unavailable", "backend unavailable");
  }

  const responseHeaders = new Headers();
  const responseContentType = response.headers.get("content-type");
  if (responseContentType) responseHeaders.set("content-type", responseContentType);

  return new Response(response.body, {
    status: response.status,
    headers: responseHeaders,
  });
}

export function GET(request: Request, context: ProxyContext) {
  return proxy(request, context);
}

export function POST(request: Request, context: ProxyContext) {
  return proxy(request, context);
}
