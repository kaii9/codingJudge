type ProxyContext = {
  params: Promise<{ path: string[] }>;
};

export function backendURL(path: string[], search: string, base = process.env.API_INTERNAL_URL ?? "") {
  if (!base) throw new Error("API_INTERNAL_URL is not configured");
  return `${base.replace(/\/$/, "")}/${path.map(encodeURIComponent).join("/")}${search}`;
}

export async function proxy(request: Request, context: ProxyContext): Promise<Response> {
  if (!process.env.API_INTERNAL_URL) {
    return Response.json(
      {
        error: {
          code: "proxy_configuration_error",
          message: "API_INTERNAL_URL is not configured",
        },
      },
      { status: 500 },
    );
  }

  const { path } = await context.params;
  const contentType = request.headers.get("content-type");
  const headers = new Headers();
  if (contentType) headers.set("content-type", contentType);

  const method = request.method.toUpperCase();
  const response = await fetch(
    backendURL(path, new URL(request.url).search, process.env.API_INTERNAL_URL),
    {
      method,
      headers,
      body: method === "GET" || method === "HEAD"
        ? undefined
        : await request.arrayBuffer(),
    },
  );

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
