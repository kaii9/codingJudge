import type { Language, SubmissionStatus } from "@/lib/types";

export type StatusVariant = "neutral" | "info" | "success" | "warning" | "danger";

export const statusMeta: Record<SubmissionStatus, { label: string; variant: StatusVariant }> = {
  queued: { label: "Queued", variant: "neutral" },
  running: { label: "Running", variant: "info" },
  accepted: { label: "Accepted", variant: "success" },
  wrong_answer: { label: "Wrong Answer", variant: "warning" },
  runtime_error: { label: "Runtime Error", variant: "danger" },
  time_limit_exceeded: { label: "Time Limit Exceeded", variant: "danger" },
  internal_error: { label: "Internal Error", variant: "danger" },
};

const terminalStatuses = new Set<SubmissionStatus>([
  "accepted", "wrong_answer", "runtime_error", "time_limit_exceeded", "internal_error",
]);

export const isTerminalStatus = (status: SubmissionStatus) => terminalStatuses.has(status);
export const draftKey = (problemId: string, language: Language) => `gojudge:draft:${problemId}:${language}`;

const templates: Record<Language, string> = {
  go: 'package main\n\nimport "fmt"\n\nfunc main() {\n\tvar a, b int\n\tfmt.Scan(&a, &b)\n\tfmt.Println(a + b)\n}\n',
  cpp: '#include <iostream>\n\nint main() {\n    long long a, b;\n    std::cin >> a >> b;\n    std::cout << a + b << "\\n";\n    return 0;\n}\n',
  python: 'def main():\n    a, b = map(int, input().split())\n    print(a + b)\n\nif __name__ == "__main__":\n    main()\n',
};

export const starterTemplate = (language: Language) => templates[language];
