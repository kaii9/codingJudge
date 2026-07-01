import { expect, test, type Locator, type Page } from "@playwright/test";

type Box = NonNullable<Awaited<ReturnType<Locator["boundingBox"]>>>;

function overlaps(a: Box, b: Box) {
  return (
    a.x < b.x + b.width &&
    a.x + a.width > b.x &&
    a.y < b.y + b.height &&
    a.y + a.height > b.y
  );
}

async function boxFor(locator: Locator) {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  return box as Box;
}

async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() => {
    const root = document.documentElement;
    const overflowedElements = Array.from(document.querySelectorAll<HTMLElement>("body *"))
      .filter(element => element.offsetParent !== null && !element.closest(".monaco-editor"))
      .map(element => {
        const rect = element.getBoundingClientRect();
        return {
          tag: element.tagName.toLowerCase(),
          className: element.className.toString(),
          left: rect.left,
          right: rect.right,
        };
      })
      .filter(({ left, right }) => left < -1 || right > root.clientWidth + 1)
      .slice(0, 10);

    return {
      clientWidth: root.clientWidth,
      scrollWidth: root.scrollWidth,
      overflowedElements,
    };
  });

  expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth + 1);
  expect(overflow.overflowedElements).toEqual([]);
}

test.describe("responsive workbench", () => {
  test("desktop panes are visible and non-overlapping", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== "chromium-desktop", "desktop-only layout assertion");

    await page.goto("/problems/sum");
    await expect(page.getByRole("heading", { name: /A\+B/ })).toBeVisible();

    const problem = await boxFor(page.locator("#workbench-problem-panel"));
    const code = await boxFor(page.locator("#workbench-code-panel"));
    const result = await boxFor(page.locator("#workbench-result-panel"));

    expect(overlaps(problem, code)).toBe(false);
    expect(overlaps(problem, result)).toBe(false);
    expect(overlaps(code, result)).toBe(false);
  });

  test("mobile uses one visible tabpanel without horizontal overflow", async ({ page }, testInfo) => {
    test.skip(testInfo.project.name !== "chromium-mobile", "mobile-only layout assertion");

    await page.goto("/problems/sum");
    await expect(page.getByRole("heading", { name: /A\+B/ })).toBeVisible();

    const problemPanel = page.getByRole("tabpanel", { name: "Problem" });
    const codePanel = page.getByRole("tabpanel", { name: "Code" });
    const resultPanel = page.getByRole("tabpanel", { name: "Result" });

    await expect(problemPanel).toBeVisible();
    await expect(codePanel).toBeHidden();
    await expect(resultPanel).toBeHidden();
    await expectNoHorizontalOverflow(page);

    await page.getByRole("tab", { name: "Code" }).click();
    await expect(problemPanel).toBeHidden();
    await expect(codePanel).toBeVisible();
    await expect(resultPanel).toBeHidden();
    await expectNoHorizontalOverflow(page);

    await page.getByRole("tab", { name: "Result" }).click();
    await expect(problemPanel).toBeHidden();
    await expect(codePanel).toBeHidden();
    await expect(resultPanel).toBeVisible();
    await expectNoHorizontalOverflow(page);
  });
});
