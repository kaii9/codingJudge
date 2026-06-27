import { StrictMode, type PropsWithChildren } from "react";
import { act, renderHook } from "@testing-library/react";
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
  expect(localStorage.getItem("gojudge:draft:sum:python")).toBe(starterTemplate("python"));
  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("package main // saved");
});
