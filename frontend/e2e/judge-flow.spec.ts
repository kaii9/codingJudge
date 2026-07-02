import { expect, test, type Page } from "@playwright/test";

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

async function fillMonaco(page: Page, code: string) {
  const input = page.locator(".monaco-editor textarea.inputarea");
  await expect(input).toBeAttached({ timeout: 15_000 });
  await input.click({ force: true });
  await input.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
  await expect.poll(() => input.evaluate(element => {
    const textarea = element as HTMLTextAreaElement;
    return textarea.selectionStart === 0 && textarea.selectionEnd === textarea.value.length;
  })).toBe(true);
  await page.keyboard.insertText(code);
}

async function openCodePane(page: Page) {
  if (await page.getByLabel("Language").isVisible()) return;

  await page.getByRole("tab", { name: "Code" }).click();
  await expect(page.getByRole("tabpanel", { name: "Code" })).toBeVisible();
}

test.describe.configure({ mode: "serial" });
test.setTimeout(60_000);

for (const language of acceptedPrograms) {
  test(`submits ${language.label} and reaches Accepted`, async ({ page }) => {
    await page.goto("/");
    const sumLink = page.getByRole("link", { name: /A\+B/ }).first();
    await expect(sumLink).toBeVisible();
    await sumLink.click();
    await expect(page.getByRole("heading", { name: /A\+B/ })).toBeVisible();
    await openCodePane(page);
    await page.getByLabel("Language").selectOption(language.value);
    await fillMonaco(page, language.code);

    await page.getByRole("button", { name: "Submit" }).click();
    await expect(
      page.getByRole("tabpanel", { name: "Result" }).getByText("Accepted"),
    ).toBeVisible({ timeout: 30_000 });

    await page.getByRole("link", { name: "Submissions" }).click();
    await expect(
      page.getByRole("row", {
        name: new RegExp(`sum\\s+${escapeRegExp(language.label)}\\s+Accepted`, "i"),
      }).first(),
    ).toBeVisible();
  });
}
