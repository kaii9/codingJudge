# Hot 20 Input Formats

The canonical IDs, stdin/stdout grammars, tie-breaking rules, and examples are defined in the Hot 20 design specification and seeded by `migrations/004_hot20_problem_set.sql`. All indices are zero-based, integer arithmetic uses signed 64-bit values where needed, and multi-answer outputs are sorted lexicographically.

Public APIs never expose the seeded test cases. The judge worker loads them from PostgreSQL only when processing a submission.
