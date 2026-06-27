"use client";

import { useCallback, useEffect, useRef, useState, type SetStateAction } from "react";
import type { Language } from "@/lib/types";
import { draftKey, starterTemplate } from "@/lib/judge";

function readDraft(key: string, language: Language) {
  try {
    return localStorage.getItem(key) ?? starterTemplate(language);
  } catch {
    return starterTemplate(language);
  }
}

function writeDraft(key: string, code: string) {
  try {
    localStorage.setItem(key, code);
  } catch {
    // The in-memory draft remains editable when browser storage is unavailable.
  }
}

export function useLocalDraft(problemId: string, language: Language) {
  const key = draftKey(problemId, language);
  const [code, setCodeState] = useState(() => starterTemplate(language));
  const activeKeyRef = useRef(key);
  const codeRef = useRef(code);

  useEffect(() => {
    const restoredCode = readDraft(key, language);
    activeKeyRef.current = key;
    codeRef.current = restoredCode;
    // Key changes intentionally hydrate local state from the corresponding draft.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setCodeState(restoredCode);
  }, [key, language]);

  const setCode = useCallback((value: SetStateAction<string>) => {
    const activeKey = activeKeyRef.current;
    const nextCode = typeof value === "function" ? value(codeRef.current) : value;
    codeRef.current = nextCode;
    writeDraft(activeKey, nextCode);
    setCodeState(nextCode);
  }, []);

  return { code, setCode };
}
