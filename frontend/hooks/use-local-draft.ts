"use client";

import { useEffect, useMemo, useState } from "react";
import type { Language } from "@/lib/types";
import { draftKey, starterTemplate } from "@/lib/judge";

export function useLocalDraft(problemId: string, language: Language) {
  const key = useMemo(() => draftKey(problemId, language), [problemId, language]);
  const [code, setCode] = useState(() => starterTemplate(language));
  const [readyKey, setReadyKey] = useState<string | null>(null);

  useEffect(() => {
    // Key changes intentionally hydrate local state before persistence is enabled.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setCode(localStorage.getItem(key) ?? starterTemplate(language));
    setReadyKey(key);
  }, [key, language]);

  useEffect(() => {
    if (readyKey === key) localStorage.setItem(key, code);
  }, [code, key, readyKey]);

  return { code, setCode };
}
