import {
  StrictMode,
  useEffect,
  useLayoutEffect,
  type Dispatch,
  type PropsWithChildren,
  type SetStateAction,
} from "react";
import { act, render, renderHook } from "@testing-library/react";
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

function PassiveEdit({ edit, setCode }: {
  edit: string | null;
  setCode: Dispatch<SetStateAction<string>>;
}) {
  useEffect(() => {
    if (edit !== null) setCode(edit);
  }, [edit, setCode]);
  return null;
}

function LayoutFunctionalEdit({ setCode, suffix }: {
  setCode: Dispatch<SetStateAction<string>>;
  suffix: string | null;
}) {
  useLayoutEffect(() => {
    if (suffix !== null) setCode(code => `${code}${suffix}`);
  }, [setCode, suffix]);
  return null;
}

function DraftWithPassiveEdit({ edit, language }: { edit: string | null; language: Language }) {
  const { code, setCode } = useLocalDraft("sum", language);
  return (
    <>
      <PassiveEdit edit={edit} setCode={setCode} />
      <output data-testid="draft-code">{code}</output>
    </>
  );
}

function DraftWithLayoutFunctionalEdit({ language, suffix }: { language: Language; suffix: string | null }) {
  const { code, setCode } = useLocalDraft("sum", language);
  return (
    <>
      <LayoutFunctionalEdit setCode={setCode} suffix={suffix} />
      <output data-testid="draft-code">{code}</output>
    </>
  );
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

it("binds a child passive-effect edit to the newly rendered key", () => {
  localStorage.setItem("gojudge:draft:sum:go", "saved go");
  localStorage.setItem("gojudge:draft:sum:python", "saved python");
  const view = render(<DraftWithPassiveEdit edit={null} language="go" />);

  view.rerender(<DraftWithPassiveEdit edit="python child edit" language="python" />);

  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("saved go");
  expect(localStorage.getItem("gojudge:draft:sum:python")).toBe("python child edit");
  expect(view.container).toHaveTextContent("python child edit");
});

it("bases a child layout-effect functional edit on the newly rendered key", () => {
  localStorage.setItem("gojudge:draft:sum:go", "saved go");
  localStorage.setItem("gojudge:draft:sum:python", "saved python");
  const view = render(<DraftWithLayoutFunctionalEdit language="go" suffix={null} />);

  view.rerender(<DraftWithLayoutFunctionalEdit language="python" suffix=" + child" />);

  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("saved go");
  expect(localStorage.getItem("gojudge:draft:sum:python")).toBe("saved python + child");
  expect(view.container).toHaveTextContent("saved python + child");
});

it("lets a stale setter persist its own key without replacing visible code", () => {
  localStorage.setItem("gojudge:draft:sum:go", "saved go");
  localStorage.setItem("gojudge:draft:sum:python", "saved python");
  const { result, rerender } = renderHook(
    ({ language }: { language: Language }) => useLocalDraft("sum", language),
    { initialProps: { language: "go" as Language } },
  );
  const staleGoSetter = result.current.setCode;

  rerender({ language: "python" });
  act(() => staleGoSetter("late go edit"));

  expect(localStorage.getItem("gojudge:draft:sum:go")).toBe("late go edit");
  expect(localStorage.getItem("gojudge:draft:sum:python")).toBe("saved python");
  expect(result.current.code).toBe("saved python");
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
