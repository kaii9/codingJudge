# Hot 20 Problem Set Design

**Date:** 2026-07-02

## Summary

Add a curated collection of 20 original, fully judgeable algorithm problems inspired by the topic coverage of common interview sets. Keep the existing `sum` and `echo` problems as a separate Starter collection, giving the repository 22 total problems while the Hot 20 collection remains exactly 20.

The change also adds normalized difficulty and tag metadata, frontend collection/search/difficulty filtering, deterministic output rules for multi-answer problems, and reference-solver validation for every hidden test case. It does not copy third-party problem statements or hidden data.

## Goals

- Provide enough problem variety to make the judge feel like a real algorithm platform rather than a two-problem demo.
- Cover arrays, hashing, two pointers, sliding windows, stacks, linked lists, trees, graphs, backtracking, and dynamic programming.
- Keep every problem compatible with complete Go, C++, and Python programs using standard input and output.
- Add metadata and filtering that remain useful when the library grows.
- Validate seed data and expected outputs automatically against reference algorithms.
- Apply cleanly to both fresh and existing PostgreSQL volumes.

## Non-Goals

- Mirroring LeetCode titles, descriptions, examples, editorial text, or test data.
- User-created problems or an administration UI.
- Server-side pagination, full-text search, or database-backed filtering for 22 records.
- Per-problem starter code, editorial solutions, hints, acceptance rates, or user progress.
- Moving test cases to MinIO.
- Adding more languages.

## Collections

The database contains two collections:

- `starter`: existing `sum` and `echo` problems.
- `hot20`: the 20 problems defined below.

The frontend defaults to Hot 20. A direct visit to a Starter problem selects the Starter collection so the active problem remains visible. The All collection view combines both.

## Problem Catalog

All integer outputs fit signed 64-bit values unless a problem says otherwise. Array indices are zero-based. Output ends with a newline; comparisons continue to use the judge's existing trailing-whitespace normalization.

### 1. Target Pair

- **ID:** `target-pair`
- **Difficulty:** Medium
- **Tags:** `array`, `hash-table`
- **Input:** `n target`, followed by `n` integers.
- **Output:** The lexicographically smallest pair `i j` with `i < j` and `a[i] + a[j] = target`; output `-1 -1` when absent.

### 2. Consecutive Streak

- **ID:** `consecutive-streak`
- **Difficulty:** Medium
- **Tags:** `array`, `hash-set`
- **Input:** `n`, followed by `n` integers in arbitrary order.
- **Output:** Length of the longest set of consecutive integer values. Duplicate values count once.

### 3. Product Without Self

- **ID:** `product-without-self`
- **Difficulty:** Medium
- **Tags:** `array`, `prefix-suffix`
- **Input:** `n`, followed by `n` integers. Every required product fits signed 64-bit.
- **Output:** `n` integers where position `i` is the product of all input values except `a[i]`, without division.

### 4. Widest Water Container

- **ID:** `widest-water-container`
- **Difficulty:** Medium
- **Tags:** `array`, `two-pointers`
- **Input:** `n`, followed by `n` non-negative heights.
- **Output:** Maximum area formed by two positions and the horizontal axis.

### 5. Zero-Sum Triples

- **ID:** `zero-sum-triples`
- **Difficulty:** Medium
- **Tags:** `array`, `sorting`, `two-pointers`
- **Input:** `n`, followed by `n` integers.
- **Output:** First output the number of unique triples. Then output one ascending triple per line; triples are ordered lexicographically. For no triples, output only `0`.

### 6. Unique Character Window

- **ID:** `unique-character-window`
- **Difficulty:** Medium
- **Tags:** `string`, `sliding-window`
- **Input:** One line containing printable ASCII characters; the line may be empty.
- **Output:** Length of the longest substring without repeated characters.

### 7. Smallest Covering Segment

- **ID:** `smallest-covering-segment`
- **Difficulty:** Hard
- **Tags:** `string`, `hash-table`, `sliding-window`
- **Input:** Source string on the first line and target string on the second line. Matching is case-sensitive and respects duplicate target characters.
- **Output:** The shortest source substring containing the target multiset. Ties choose the earliest start. Output `-1` when absent; an empty target produces an empty line.

### 8. Balanced Delimiters

- **ID:** `balanced-delimiters`
- **Difficulty:** Easy
- **Tags:** `string`, `stack`
- **Input:** One string containing only `()[]{}` characters; it may be empty.
- **Output:** `true` when every delimiter is correctly nested, otherwise `false`.

### 9. Warmer Day Distance

- **ID:** `warmer-day-distance`
- **Difficulty:** Medium
- **Tags:** `array`, `monotonic-stack`
- **Input:** `n`, followed by `n` daily temperatures.
- **Output:** `n` distances to the next strictly warmer day, or `0` where none exists.

### 10. Largest Skyline Rectangle

- **ID:** `largest-skyline-rectangle`
- **Difficulty:** Hard
- **Tags:** `array`, `monotonic-stack`
- **Input:** `n`, followed by `n` non-negative bar heights of unit width.
- **Output:** Maximum rectangular area contained in the histogram.

### 11. Reverse Node Chain

- **ID:** `reverse-node-chain`
- **Difficulty:** Easy
- **Tags:** `linked-list`, `iteration`
- **Input:** `n`, followed by `n` node values in traversal order.
- **Output:** Values after reversing the chain. An empty chain produces an empty line.

### 12. Merge Sorted Chains

- **ID:** `merge-sorted-chains`
- **Difficulty:** Easy
- **Tags:** `linked-list`, `two-pointers`
- **Input:** `n m`, one line of `n` sorted values, then one line of `m` sorted values. Either chain may be empty.
- **Output:** Values of the merged non-decreasing chain.

### 13. Cycle Entry

- **ID:** `cycle-entry`
- **Difficulty:** Medium
- **Tags:** `linked-list`, `two-pointers`
- **Input:** `n`, followed by `n` next-node indices. Nodes are `0..n-1`, `-1` means null, and traversal begins at node `0` when `n > 0`.
- **Output:** Index where the reachable cycle begins, or `-1` when the traversal terminates.

### 14. Tree Level Traversal

- **ID:** `tree-level-traversal`
- **Difficulty:** Medium
- **Tags:** `binary-tree`, `breadth-first-search`
- **Input:** Token count `n`, followed by `n` level-order tokens. Each token is an integer or `null`; children are assigned in queue order.
- **Output:** Number of non-empty levels, followed by one line of node values per level. An empty tree outputs only `0`.

### 15. Search Tree Validation

- **ID:** `search-tree-validation`
- **Difficulty:** Medium
- **Tags:** `binary-tree`, `depth-first-search`, `binary-search-tree`
- **Input:** The same level-order encoding as Tree Level Traversal.
- **Output:** `true` only for a strict binary search tree; duplicate keys are invalid.

### 16. Archipelago Count

- **ID:** `archipelago-count`
- **Difficulty:** Medium
- **Tags:** `matrix`, `depth-first-search`, `breadth-first-search`
- **Input:** `rows cols`, followed by `rows` strings of `0` and `1` with length `cols`.
- **Output:** Number of four-directionally connected land components.

### 17. Course Dependency Order

- **ID:** `course-dependency-order`
- **Difficulty:** Medium
- **Tags:** `graph`, `topological-sort`, `heap`
- **Input:** `n m`, followed by `m` pairs `course prerequisite`, representing `prerequisite -> course`.
- **Output:** The lexicographically smallest valid ordering of all courses `0..n-1`; output `-1` when a cycle prevents completion.

### 18. Unique Target Combinations

- **ID:** `unique-target-combinations`
- **Difficulty:** Medium
- **Tags:** `array`, `backtracking`
- **Input:** `n target`, followed by `n` distinct positive candidate values. Each candidate may be reused.
- **Output:** Number of combinations, followed by one non-decreasing combination per line. Combinations are ordered lexicographically. For no combinations, output only `0`.

### 19. Maximum Non-Adjacent Sum

- **ID:** `maximum-non-adjacent-sum`
- **Difficulty:** Medium
- **Tags:** `array`, `dynamic-programming`
- **Input:** `n`, followed by `n` non-negative values.
- **Output:** Maximum sum obtainable without selecting adjacent positions. An empty array outputs `0`.

### 20. Minimum Coin Count

- **ID:** `minimum-coin-count`
- **Difficulty:** Medium
- **Tags:** `array`, `dynamic-programming`
- **Input:** `n amount`, followed by `n` distinct positive coin values.
- **Output:** Minimum number of coins needed for the amount with unlimited reuse, or `-1` when impossible. Amount zero outputs `0`.

## Database Design

Create `migrations/004_hot20_problem_set.sql`.

Add metadata columns to `problems`:

```sql
difficulty TEXT NOT NULL DEFAULT 'easy'
collection TEXT NOT NULL DEFAULT 'starter'
sort_order INTEGER NOT NULL DEFAULT 0
```

Add check constraints for the three difficulty values and two collection values. Existing `sum` and `echo` remain Starter problems and receive explicit sort positions.

Normalize tags in a join table:

```sql
CREATE TABLE problem_tags (
    problem_id TEXT NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    PRIMARY KEY (problem_id, tag)
);
```

Add an index on `(collection, sort_order, id)` and an index on `(tag, problem_id)`.

The migration is safe for existing volumes:

1. Add metadata columns and constraints idempotently.
2. Upsert Starter metadata.
3. Upsert all 20 Hot problems by stable ID.
4. Replace tags for the 22 managed problems.
5. Delete and recreate test cases only for the 20 Hot IDs.

Running the migration twice must not duplicate tags or cases. A fresh database continues to apply migrations in filename order. Add `make migrate-hot20` for existing volumes.

## Domain and Store Changes

Add domain types:

```text
ProblemDifficulty: easy | medium | hard
ProblemCollection: starter | hot20
```

Extend `Problem` with `Difficulty`, `Collection`, `SortOrder`, and `Tags`. These fields are public metadata. `TestCases` remains omitted from HTTP responses.

PostgreSQL list/detail queries load tags in deterministic alphabetical order. Problem list order is:

1. Hot 20 before Starter.
2. `sort_order` within each collection.
3. Stable ID as final tie-breaker.

The in-memory sample problems receive the same Starter metadata so unit-mode API responses match PostgreSQL shape.

## API Contract

`GET /problems` and `GET /problems/{id}` add:

```json
{
  "difficulty": "medium",
  "collection": "hot20",
  "sortOrder": 1,
  "tags": ["array", "hash-table"]
}
```

No query parameters are added. Filtering remains client-side at this collection size. OpenAPI schemas and examples are updated. Hidden `testCases` remain absent from every public response.

## Frontend Behavior

Extend the TypeScript `Problem` contract with exact difficulty, collection, order, and tag unions.

Convert the problem browser into a client-side filtered rail while preserving its existing role in the workbench:

- Collection segmented control: Hot 20, Starter, All.
- Search input matching title or tag, case-insensitively.
- Difficulty select: All difficulties, Easy, Medium, Hard.
- Search, collection, and difficulty filters compose with each other.
- Empty results render a compact `No matching problems.` state.
- Each list row has stable dimensions and shows title, difficulty, and up to the available compact tags without resizing on hover.
- The active problem determines the initial collection. Hot 20 is used when no active problem exists.
- Changing route to a problem in another collection updates the collection filter so the active item remains visible.
- The problem statement header displays difficulty and all tags.
- The problem list scrolls within the existing rail. The page and editor dimensions do not shift when filters change.

Mobile behavior remains inside the existing Problem tab. Controls wrap without horizontal overflow, text does not overlap, and the code/result tabs are unchanged.

## Seed Data Quality

Each Hot problem contains at least six hidden cases. Cases cover:

- minimum and empty inputs where the format permits them,
- duplicates and repeated characters,
- no-solution paths,
- tie-breaking rules,
- zero values and negative values where allowed,
- skewed trees and disconnected graphs,
- values large enough to require 64-bit arithmetic,
- representative normal cases.

Descriptions include explicit Input, Output, and Example sections in plain text. They explain the deterministic ordering rules for multi-answer output. They do not mention or reproduce third-party problem text.

## Reference Validation

Add an integration-tagged Hot 20 validation test that:

1. Applies all migration files in sorted order to an isolated PostgreSQL test database.
2. Queries the 20 Hot problems, tags, and all test cases.
3. Dispatches each problem ID to a small trusted Go reference solver.
4. Parses the seeded standard input.
5. Compares normalized reference output with the seeded expected output.

The test fails on an unknown problem ID, malformed input, ambiguous output ordering, incorrect expected output, fewer than six cases, duplicate metadata, or a collection count other than 20.

Existing PostgreSQL integration setup will be changed from a hard-coded migration list to sorted migration discovery so future migrations are included automatically.

## Testing Strategy

### Backend unit and integration tests

- Difficulty and collection values serialize exactly.
- Memory and PostgreSQL stores return sorted tags and stable problem order.
- List/detail HTTP responses include metadata and exclude test cases.
- Migration is idempotent when applied twice.
- Exactly 20 `hot20` and two `starter` records exist.
- Every Hot problem has two to four tags and at least six cases.
- Reference solvers validate every expected output.

### Frontend tests

- Hot 20 is the default collection without an active Starter problem.
- Direct Starter navigation selects Starter.
- Search matches titles and tags case-insensitively.
- Collection and difficulty filters compose.
- Empty state renders correctly.
- Difficulty and tags appear in the problem list and statement.
- Existing recent submissions and active-link behavior remain intact.
- Desktop and mobile controls do not overflow or overlap.

### Compose acceptance

- Apply migration to the existing development volume without deleting data.
- Confirm the UI shows 20 Hot problems plus two Starter problems.
- Judge at least four representative categories: hash table, tree, graph, and dynamic programming.
- Across those submissions, use Go, C++, and Python at least once.
- Confirm Redis Pending returns to zero.
- Re-run existing multi-worker fault recovery after the larger seed is installed.

## Documentation

Update README with:

- Hot 20 collection overview and topic coverage.
- `make migrate-hot20` for existing volumes.
- filtering behavior and total problem counts.
- verification commands for seed/reference tests.

Update the development plan to mark the curated problem-library slice complete. Keep the next backend priority as observability and measured load testing.

## Acceptance Criteria

- The Hot 20 collection contains exactly 20 original problems with stable IDs.
- Starter remains a separate two-problem collection.
- Every Hot problem has difficulty, ordered tags, a complete stdin/stdout description, and at least six validated hidden cases.
- Public API metadata is complete and hidden tests remain private.
- The frontend can search and combine collection/difficulty filters on desktop and mobile.
- Existing submissions, judging, Outbox, leases, multi-worker recovery, and frontend workflows do not regress.
- Unit, race, vet, PostgreSQL/Redis integration, reference-solver, frontend, build, Playwright, Compose, and fault-recovery checks pass.
