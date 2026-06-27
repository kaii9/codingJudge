import { describe, expect, it } from "vitest";
import { draftKey, isTerminalStatus, starterTemplate, statusMeta } from "@/lib/judge";

describe("judge helpers", () => {
  it.each(["accepted", "wrong_answer", "runtime_error", "time_limit_exceeded", "internal_error"] as const)(
    "treats %s as terminal", status => expect(isTerminalStatus(status)).toBe(true),
  );
  it.each(["queued", "running"] as const)(
    "keeps polling %s", status => expect(isTerminalStatus(status)).toBe(false),
  );
  it("uses a problem/language-specific draft key", () => {
    expect(draftKey("sum", "python")).toBe("gojudge:draft:sum:python");
  });
  it("provides language entrypoints and A+B starter content", () => {
    expect(starterTemplate("go")).toContain("package main");
    expect(starterTemplate("go")).toContain("fmt.Println(a + b)");
    expect(starterTemplate("cpp")).toContain("#include <iostream>");
    expect(starterTemplate("cpp")).toContain("std::cout << a + b");
    expect(starterTemplate("python")).toContain("def main");
    expect(starterTemplate("python")).toContain("print(a + b)");
  });
  it.each([
    ["queued", "Queued", "neutral"],
    ["running", "Running", "info"],
    ["accepted", "Accepted", "success"],
    ["wrong_answer", "Wrong Answer", "warning"],
    ["runtime_error", "Runtime Error", "danger"],
    ["time_limit_exceeded", "Time Limit Exceeded", "danger"],
    ["internal_error", "Internal Error", "danger"],
  ] as const)("describes %s", (status, label, variant) => {
    expect(statusMeta[status]).toEqual({ label, variant });
  });
});
