"use client";

import dynamic from "next/dynamic";
import { Play } from "lucide-react";
import { useCallback, useRef, useState, type CSSProperties } from "react";
import type { EditorProps } from "@monaco-editor/react";
import { useLocalDraft } from "@/hooks/use-local-draft";
import type { CreateSubmissionInput, Language } from "@/lib/types";

type WorkspaceSubmission = Pick<CreateSubmissionInput, "language" | "code">;

export interface CodeWorkspaceProps {
  problemId: string;
  submitting: boolean;
  onSubmit: (input: WorkspaceSubmission) => Promise<void>;
}

const languages: ReadonlyArray<{ id: Language; label: string; monacoId: string }> = [
  { id: "go", label: "Go", monacoId: "go" },
  { id: "cpp", label: "C++", monacoId: "cpp" },
  { id: "python", label: "Python", monacoId: "python" },
];

const monacoLanguage: Record<Language, string> = {
  go: "go",
  cpp: "cpp",
  python: "python",
};

const fileExtension: Record<Language, string> = {
  go: "go",
  cpp: "cpp",
  python: "py",
};

function encodeModelSegment(value: string) {
  return encodeURIComponent(value).replace(/[.!'()*]/g, character =>
    `%${character.charCodeAt(0).toString(16).toUpperCase()}`,
  );
}

function editorModelPath(problemId: string, language: Language) {
  return `gojudge://draft/${encodeModelSegment(problemId)}/main.${fileExtension[language]}`;
}

const editorOptions = {
  accessibilitySupport: "auto",
  ariaLabel: "Code editor",
  automaticLayout: true,
  editContext: false,
  fontSize: 14,
  minimap: { enabled: false },
  scrollBeyondLastLine: false,
} satisfies NonNullable<EditorProps["options"]>;

const editorFrameStyle: CSSProperties = {
  width: "100%",
  height: "32rem",
  minHeight: "22rem",
  maxHeight: "42rem",
  overflow: "hidden",
  background: "#1e1e1e",
  border: "1px solid #344258",
  borderRadius: "6px",
};

const skeletonStyle: CSSProperties = {
  width: "100%",
  height: "100%",
  background: "#1e1e1e",
};

const submitStyle: CSSProperties = {
  minWidth: "8.5rem",
  height: "2.5rem",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: "0.5rem",
  padding: "0 1rem",
  color: "#ffffff",
  backgroundColor: "#d83a43",
  border: 0,
  borderRadius: "6px",
  fontSize: "0.875rem",
  fontWeight: 700,
  cursor: "pointer",
};

function EditorLoadingSkeleton() {
  return (
    <div
      className="code-workspace__editor-skeleton"
      role="status"
      aria-label="Loading code editor"
      style={skeletonStyle}
    />
  );
}

const MonacoEditor = dynamic(
  () => import("@monaco-editor/react").then(module => module.default),
  {
    ssr: false,
    loading: EditorLoadingSkeleton,
  },
);

export function CodeWorkspace({ problemId, submitting, onSubmit }: CodeWorkspaceProps) {
  const [language, setLanguage] = useState<Language>("go");
  const [locallySubmitting, setLocallySubmitting] = useState(false);
  const { code, setCode } = useLocalDraft(problemId, language);
  const submissionInFlightRef = useRef(false);
  const isSubmitting = submitting || locallySubmitting;
  const submitDisabled = isSubmitting || code.trim().length === 0;

  const handleEditorChange = useCallback((value?: string) => {
    if (value !== undefined) setCode(value);
  }, [setCode]);

  async function handleSubmit() {
    if (submitDisabled || submissionInFlightRef.current) return;

    submissionInFlightRef.current = true;
    setLocallySubmitting(true);
    try {
      await onSubmit({ language, code });
    } catch {
      // The parent owns user-facing submission errors; do not leak a rejected event promise.
    } finally {
      submissionInFlightRef.current = false;
      setLocallySubmitting(false);
    }
  }

  return (
    <section className="code-workspace" aria-labelledby="code-workspace-heading">
      <header className="code-workspace__toolbar">
        <h2 id="code-workspace-heading">Code</h2>
        <label htmlFor="code-workspace-language">Language</label>
        <select
          id="code-workspace-language"
          value={language}
          onChange={event => setLanguage(event.currentTarget.value as Language)}
        >
          {languages.map(option => (
            <option key={option.id} value={option.id}>{option.label}</option>
          ))}
        </select>
      </header>

      <div className="code-workspace__editor" style={editorFrameStyle}>
        <MonacoEditor
          height="100%"
          language={monacoLanguage[language]}
          options={editorOptions}
          path={editorModelPath(problemId, language)}
          theme="vs-dark"
          value={code}
          onChange={handleEditorChange}
        />
      </div>

      <footer className="code-workspace__actions">
        <button
          type="button"
          disabled={submitDisabled}
          aria-busy={isSubmitting}
          style={{
            ...submitStyle,
            cursor: submitDisabled ? "not-allowed" : submitStyle.cursor,
            opacity: submitDisabled ? 0.65 : 1,
          }}
          onClick={handleSubmit}
        >
          <Play size={16} aria-hidden="true" />
          <span>{isSubmitting ? "Submitting..." : "Submit"}</span>
        </button>
      </footer>
    </section>
  );
}
