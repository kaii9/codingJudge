import { StrictMode, type PropsWithChildren } from "react";
import { act, renderHook } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { afterAll, beforeAll, beforeEach, expect, it, vi } from "vitest";
import { useLocalDraft } from "@/hooks/use-local-draft";
import { starterTemplate } from "@/lib/judge";
import type { Language } from "@/lib/types";

beforeAll(() => {
  const environment = globalThis as typeof globalThis & { jsdom: { window: Window } };
  vi.stubGlobal("localStorage", environment.jsdom.window.localStorage);
});
afterAll(() => vi.unstubAllGlobals());
beforeEach(() => localStorage.clear());

function DraftProbe() {
  const { code } = useLocalDraft("sum", "go");
  return <pre>{code}</pre>;
}

it("restores an existing draft without replacing it with a starter", () => {
  localStorage.setItem("gojudge:draft:sum:go", "package main // saved");
  const { result } = renderHook(() => useLocalDraft("sum", "go"));
  expect(result.current.code).toBe("package main // saved");
  act(() => result.current.setCode("package main // changed"));
  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("package main // changed");
});

it("does not write the previous language draft into a new key in StrictMode", () => {
  localStorage.setItem("gojudge:draft:sum:go", "package main // saved");
  const wrapper = ({ children }: PropsWithChildren) => <StrictMode>{children}</StrictMode>;
  const { result, rerender } = renderHook(
    ({ language }: { language: Language }) => useLocalDraft("sum", language),
    { initialProps: { language: "go" as Language }, wrapper },
  );

  rerender({ language: "python" });

  expect(result.current.code).toBe(starterTemplate("python"));
  expect(localStorage.getItem("gojudge:draft:sum:python")).toBeNull();
  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("package main // saved");
});

it("keeps the outgoing edit when code and language change in one batch", () => {
  localStorage.setItem("gojudge:draft:sum:go", "saved go");
  localStorage.setItem("gojudge:draft:sum:python", "saved python");
  const { result, rerender } = renderHook(
    ({ language }: { language: Language }) => useLocalDraft("sum", language),
    { initialProps: { language: "go" as Language } },
  );

  act(() => {
    result.current.setCode("latest go");
    rerender({ language: "python" });
  });

  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("latest go");
  expect(result.current.code).toBe("saved python");
  expect(localStorage.getItem("gojudge:draft:sum:python")).toBe("saved python");
});

it("falls back to the starter and remains editable when reading storage throws", () => {
  const getItem = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new DOMException("storage denied", "SecurityError");
  });

  const { result } = renderHook(() => useLocalDraft("sum", "go"));

  expect(result.current.code).toBe(starterTemplate("go"));
  act(() => result.current.setCode("editable in memory"));
  expect(result.current.code).toBe("editable in memory");
  expect(getItem).toHaveBeenCalled();
});

it("remains editable when writing storage exceeds quota", () => {
  const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("storage full", "QuotaExceededError");
  });

  const { result } = renderHook(() => useLocalDraft("sum", "go"));

  act(() => result.current.setCode("editable in memory"));
  expect(result.current.code).toBe("editable in memory");
  expect(setItem).toHaveBeenCalled();
});

it("renders a starter during SSR without reading browser storage", () => {
  const getItem = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new DOMException("storage unavailable", "SecurityError");
  });

  const html = renderToString(<DraftProbe />);

  expect(html).toContain("package main");
  expect(getItem).not.toHaveBeenCalled();
});
