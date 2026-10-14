import assert from "node:assert/strict";
import test from "node:test";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { ESLint } from "eslint";

// Exercise the installed linter and project configuration, not a copy of its rules.
const eslint = new ESLint();

const rejected = [
  ["conditional Hooks", 'import { useState } from "react"; export default function Page({ enabled }) { if (enabled) useState(0); return null; }', "react-hooks/rules-of-hooks"],
  ["render-time state update", 'import { useState } from "react"; export default function Page() { const [value, setValue] = useState(0); setValue(1); return <p>{value}</p>; }', "react-hooks/set-state-in-render"],
  ["missing list keys", 'export default function Page() { return [1, 2].map(value => <p>{value}</p>); }', "react/jsx-key"],
  ["explicit any", 'export const value: any = 1;', "@typescript-eslint/no-explicit-any"],
  ["internal anchor navigation", 'export default function Page() { return <a href="/submissions">History</a>; }', "no-restricted-syntax"],
  ["expression-valued internal anchor", 'export default function Page() { return <a href={"/problems/1"}>Problem</a>; }', "no-restricted-syntax"],
  ["relative internal anchor", 'export default function Page() { return <a href="submissions">History</a>; }', "no-restricted-syntax"],
  ["template-valued internal anchor", 'export default function Page({ id }: { id: string }) { return <a href={`/problems/${id}`}>Problem</a>; }', "no-restricted-syntax"],
  ["reserved module binding", 'export const module = 1;', "no-restricted-syntax"],
  ["legacy Document import", 'import Document from "next/document"; export default Document;', "no-restricted-imports"],
  ["legacy Head import", 'import Head from "next/head"; export default Head;', "no-restricted-imports"],
];

for (const [name, source, rule] of rejected) {
  test(`lint rejects ${name}`, async () => {
    const [result] = await eslint.lintText(source, { filePath: "app/lint-contract.tsx" });
    assert.ok(result.messages.some(message => message.ruleId === rule && message.severity === 2), JSON.stringify(result.messages));
    assert.ok(result.errorCount > 0);
  });
}

test("accessibility and effect-dependency warnings remain visible", async () => {
  const [result] = await eslint.lintText('import { useEffect } from "react"; export default function Page({ value }: { value: string }) { useEffect(() => console.log(value), []); return <img src="/icon.png" />; }', { filePath: "app/lint-contract.tsx" });
  for (const rule of ["jsx-a11y/alt-text", "react-hooks/exhaustive-deps"]) {
    assert.ok(result.messages.some(message => message.ruleId === rule && message.severity === 1), JSON.stringify(result.messages));
  }
});

function biomeLint(source) {
  const directory = mkdtempSync(path.join(tmpdir(), "gojudge-lint-contract-"));
  try {
    const fixture = path.join(directory, "page.tsx");
    writeFileSync(fixture, source);
    const result = spawnSync(process.execPath, [
      fileURLToPath(new URL("../node_modules/@biomejs/biome/bin/biome", import.meta.url)),
      "lint", "--config-path=.", fixture,
    ], { encoding: "utf8", timeout: 30_000 });
    assert.ifError(result.error);
    return { status: result.status, output: result.stdout + result.stderr };
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
}

for (const [name, source, rule] of [
  ["synchronous third-party script", 'export default function Page() { return <script src="https://example.com/script.js" />; }', "lint/performance/noSyncScripts"],
  ["inline Next script without id", 'import Script from "next/script"; export default function Page() { return <Script>{"console.log(1)"}</Script>; }', "lint/correctness/useInlineScriptId"],
  ["inline HTML in aliased Next script without id", 'import NextScript from "next/script"; export default function Page() { return <NextScript dangerouslySetInnerHTML={{ __html: "console.log(1)" }} />; }', "lint/correctness/useInlineScriptId"],
  ["async client component", '"use client"; export default async function Page() { return <p>Invalid</p>; }', "lint/correctness/noNextAsyncClientComponent"],
]) {
  test(`Next lint rejects ${name}`, () => {
    const result = biomeLint(source);
    assert.match(result.output, new RegExp(rule));
    assert.equal(result.status, 1, result.output);
  });
}

for (const [name, source] of [
  ["async server component", 'export default async function Page() { return <p>Server</p>; }'],
  ["synchronous client component", '"use client"; export default function Page() { return <p>Client</p>; }'],
  ["identified inline Next script", 'import Script from "next/script"; export default function Page() { return <Script id="analytics">{"console.log(1)"}</Script>; }'],
  ["deferred third-party script", 'export default function Page() { return <script src="https://example.com/script.js" defer />; }'],
]) {
  test(`Next lint accepts ${name}`, () => {
    const result = biomeLint(source);
    assert.equal(result.status, 0, result.output);
  });
}

for (const [name, source, rule] of [
  ["unoptimized img", 'export default function Page() { return <img src="/icon.png" alt="Icon" />; }', "lint/performance/noImgElement"],
  ["duplicate polyfills", 'export default function Page() { return <script defer src="https://polyfill.io/v3/polyfill.min.js?features=Object.fromEntries" />; }', "lint/performance/noUnwantedPolyfillio"],
  ["missing font display strategy", 'export default function Page() { return <link href="https://fonts.googleapis.com/css2?family=Krona+One" />; }', "lint/suspicious/useGoogleFontDisplay"],
]) {
  test(`Next lint reports ${name}`, () => {
    const result = biomeLint(source);
    assert.match(result.output, new RegExp(rule));
    assert.equal(result.status, 0, result.output);
  });
}

test("valid App Router navigation, downloads and external links are accepted", async () => {
  const [result] = await eslint.lintText('import Link from "next/link"; export default function Page({ id }: { id: string }) { return <><Link href="/submissions">History</Link><a href="/file.zip" download>Download</a><a href="https://example.com">External</a><a href="#section">Section</a><a href="//example.com">External</a><a href="mailto:test@example.com">Mail</a><a href={`https://example.com/${id}/profile`}>External</a></>; }', { filePath: "app/lint-contract.tsx" });
  assert.equal(result.errorCount, 0, JSON.stringify(result.messages));
});

test("escaped external URL templates are accepted", async () => {
  const [result] = await eslint.lintText('export default function Page() { return <a href={`\\x68ttps://example.com`}>External</a>; }', { filePath: "app/lint-contract.tsx" });
  assert.equal(result.errorCount, 0, JSON.stringify(result.messages));
});
