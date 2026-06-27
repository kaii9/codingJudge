import { expect, it } from "vitest";
import { backendURL } from "@/app/api/[...path]/route";

it("preserves path and query without exposing the internal URL", () => {
  expect(backendURL(["submissions", "sub-1"], "?verbose=1", "http://api:8080"))
    .toBe("http://api:8080/submissions/sub-1?verbose=1");
});
