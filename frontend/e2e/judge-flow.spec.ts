import { expect, test, type Page } from "@playwright/test";
import type * as Monaco from "monaco-editor";

type LanguageCase = {
  label: string;
  value: "go" | "cpp" | "python";
  code: string;
};

const acceptedPrograms: LanguageCase[] = [
  {
    label: "Go",
    value: "go",
    code: 'package main\n\nimport "fmt"\n\nfunc main() {\n\tvar a, b int\n\tfmt.Scan(&a, &b)\n\tfmt.Println(a + b)\n}\n',
  },
  {
    label: "C++",
    value: "cpp",
    code: '#include <iostream>\n\nint main() {\n    long long a, b;\n    std::cin >> a >> b;\n    std::cout << a + b << "\\n";\n    return 0;\n}\n',
  },
  {
    label: "Python",
    value: "python",
    code: "a, b = map(int, input().split())\nprint(a + b)\n",
  },
];

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function monacoState(page: Page) {
  return page.evaluate(() => {
    const monaco = (window as unknown as { monaco?: typeof Monaco }).monaco;
    const editor = monaco?.editor.getEditors()[0];
    const model = editor?.getModel();
    const selection = editor?.getSelection();
    return {
      language: model?.getLanguageId(),
      code: model?.getValue(),
      selectedAll: !!model && !!selection
        && selection.startLineNumber === 1 && selection.startColumn === 1
        && selection.endLineNumber === model.getLineCount()
        && selection.endColumn === model.getLineMaxColumn(model.getLineCount()),
    };
  });
}

async function fillMonaco(page: Page, language: LanguageCase["value"], code: string) {
  // Switching the select schedules a different Monaco model. An attached
  // textarea (often empty/virtualized) cannot prove the new model is ready.
  await expect.poll(async () => (await monacoState(page)).language).toBe(language);
  const input = page.locator(".monaco-editor textarea.inputarea");
  await expect(input).toBeAttached({ timeout: 15_000 });
  await input.click({ force: true });
  await expect(input).toBeFocused();
  // Monaco chooses browser keybindings from navigator.userAgent, not the OS
  // running Node. Desktop Chrome emulates Windows even on a macOS runner.
  const selectAll = await page.evaluate(() => navigator.userAgent.includes("Macintosh") ? "Meta+A" : "Control+A");
  await input.press(selectAll);
  await expect.poll(async () => (await monacoState(page)).selectedAll).toBe(true);
  await page.keyboard.insertText(code);
  // Monaco auto-indents typed multiline Go/C++; those spaces are insignificant.
  // Python indentation is semantic, so retain its exact bytes in this check.
  const comparable = (value: string | undefined) => language === "python"
    ? value : value?.replace(/[ \t]/g, "");
  await expect.poll(async () => comparable((await monacoState(page)).code)).toBe(comparable(code));
  return (await monacoState(page)).code!;
}

async function openCodePane(page: Page) {
  if (await page.getByLabel("Language").isVisible()) return;

  await page.getByRole("tab", { name: "Code" }).click();
  await expect(page.getByRole("tabpanel", { name: "Code" })).toBeVisible();
}

async function registerTestUser(page: Page, suffix: string) {
  const username = `e2e_${suffix}_${Date.now().toString(36)}`.slice(0, 32);
  await page.getByPlaceholder("username").fill(username);
  await page.getByPlaceholder("password").fill("correct-password");
  await page.getByRole("button", { name: "Register" }).click();
  await expect(page.getByRole("button", { name: "Log out" })).toBeVisible();
}

test.describe.configure({ mode: "serial" });
// A Go compile alone may use 30s; each case also has bounded Docker lifecycle
// time. Match the existing Executor RPC budget, not just the compile budget.
// This is a functional test deadline, not a performance SLO or sandbox limit.
const verdictTimeoutMs = 120_000;
test.setTimeout(verdictTimeoutMs + 30_000);

for (const language of acceptedPrograms) {
  test(`submits ${language.label} and reaches Accepted`, async ({ page }) => {
    await page.goto("/");
    await registerTestUser(page, language.value);
    await page.goto("/problems/sum");
    await expect(page.getByRole("heading", { name: /A\+B/ })).toBeVisible();
    await openCodePane(page);
    await page.getByLabel("Language").selectOption(language.value);
    const enteredCode = await fillMonaco(page, language.value, language.code);

    const submitted = page.waitForRequest(request => request.method() === "POST"
      && new URL(request.url()).pathname === "/api/submissions");
    await page.getByRole("button", { name: "Submit" }).click();
    expect((await submitted).postDataJSON()).toMatchObject({ language: language.value, code: enteredCode });
    const status = page.getByRole("tabpanel", { name: "Result" }).getByRole("status");
    await expect(status).toContainText(/Accepted|Wrong Answer|Compile Error|Runtime Error|Time Limit Exceeded|Internal Error/, { timeout: verdictTimeoutMs });
    await expect(status).toContainText("Accepted");

    await page.getByRole("link", { name: "Submissions" }).click();
    await expect(
      page.getByRole("row", {
        name: new RegExp(`sum\\s+${escapeRegExp(language.label)}\\s+Accepted`, "i"),
      }).first(),
    ).toBeVisible();
  });
}
