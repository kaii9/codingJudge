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

function latestDraft(drafts: Map<string, string>, key: string, language: Language) {
  if (drafts.has(key)) return drafts.get(key)!;
  const code = readDraft(key, language);
  drafts.set(key, code);
  return code;
}

export function useLocalDraft(problemId: string, language: Language) {
  const key = draftKey(problemId, language);
  const [visibleDraft, setVisibleDraft] = useState(() => ({
    key,
    code: starterTemplate(language),
  }));
  const latestCodeByKeyRef = useRef(new Map<string, string>());

  if (visibleDraft.key !== key) {
    setVisibleDraft({ key, code: starterTemplate(language) });
  }

  useEffect(() => {
    const restoredCode = latestDraft(latestCodeByKeyRef.current, key, language);
    setVisibleDraft(current => current.key === key
      ? { key, code: restoredCode }
      : current);
  }, [key, language]);

  const setCode = useCallback((value: SetStateAction<string>) => {
    const nextCode = typeof value === "function"
      ? value(latestDraft(latestCodeByKeyRef.current, key, language))
      : value;
    latestCodeByKeyRef.current.set(key, nextCode);
    writeDraft(key, nextCode);
    setVisibleDraft(current => current.key === key
      ? { key, code: nextCode }
      : current);
  }, [key, language]);

  return { code: visibleDraft.code, setCode };
}
