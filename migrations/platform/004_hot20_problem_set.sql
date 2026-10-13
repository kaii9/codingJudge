ALTER TABLE problems
    ADD COLUMN IF NOT EXISTS difficulty TEXT NOT NULL DEFAULT 'easy',
    ADD COLUMN IF NOT EXISTS collection TEXT NOT NULL DEFAULT 'starter',
    ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;

DO $$ BEGIN
    ALTER TABLE problems ADD CONSTRAINT problems_difficulty_check
        CHECK (difficulty IN ('easy', 'medium', 'hard'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE problems ADD CONSTRAINT problems_collection_check
        CHECK (collection IN ('starter', 'hot20'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS problem_tags (
    problem_id TEXT NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    PRIMARY KEY (problem_id, tag)
);

CREATE INDEX IF NOT EXISTS problems_collection_order_idx
    ON problems (collection, sort_order, id);
CREATE INDEX IF NOT EXISTS problem_tags_tag_idx
    ON problem_tags (tag, problem_id);

UPDATE problems SET difficulty = 'easy', collection = 'starter', sort_order = 1 WHERE id = 'sum';
UPDATE problems SET difficulty = 'easy', collection = 'starter', sort_order = 2 WHERE id = 'echo';

INSERT INTO problems (id, title, description, language, time_limit_ms, memory_limit_mb, difficulty, collection, sort_order)
VALUES
('target-pair','Target Pair','Find the lexicographically smallest zero-based pair whose values sum to the target. Input: n target, then n integers. Output: i j, or -1 -1. Example: 4 9 / 2 7 11 15 -> 0 1.','go',2000,128,'medium','hot20',1),
('consecutive-streak','Consecutive Streak','Find the longest run of consecutive values in an unordered array; duplicates count once. Input: n, then n integers. Output: the run length.','go',2000,128,'medium','hot20',2),
('product-without-self','Product Without Self','For each position print the product of every other value without division. Input: n, then n integers. Output: n signed 64-bit products.','go',2000,128,'medium','hot20',3),
('widest-water-container','Widest Water Container','Choose two heights maximizing width times the shorter height. Input: n, then n non-negative heights. Output: maximum area.','go',2000,128,'medium','hot20',4),
('zero-sum-triples','Zero-Sum Triples','List unique triples summing to zero. Output the count then ascending triples in lexicographic order. Input: n, then n integers.','go',2500,128,'medium','hot20',5),
('unique-character-window','Unique Character Window','Find the longest substring with no repeated printable ASCII character. Input: one line. Output: maximum length.','go',2000,128,'medium','hot20',6),
('smallest-covering-segment','Smallest Covering Segment','Find the earliest shortest source substring containing the target character multiset. Input: source line, target line. Output: substring, empty line for empty target, or -1.','go',2500,128,'hard','hot20',7),
('balanced-delimiters','Balanced Delimiters','Check whether a string of round, square and curly delimiters is correctly nested. Input: one line. Output: true or false.','go',1500,64,'easy','hot20',8),
('warmer-day-distance','Warmer Day Distance','For each temperature print days until a strictly warmer value, or zero. Input: n, then n temperatures. Output: n distances.','go',2000,128,'medium','hot20',9),
('largest-skyline-rectangle','Largest Skyline Rectangle','Find the largest unit-width rectangle inside a histogram. Input: n, then n heights. Output: maximum area.','go',2500,128,'hard','hot20',10),
('reverse-node-chain','Reverse Node Chain','Reverse values of a singly traversed chain. Input: n, then n values. Output: reversed values; an empty chain prints an empty line.','go',1500,64,'easy','hot20',11),
('merge-sorted-chains','Merge Sorted Chains','Merge two non-decreasing node chains. Input: n m, then each chain values. Output: merged values.','go',1500,64,'easy','hot20',12),
('cycle-entry','Cycle Entry','Nodes are indices and input gives each next index; traversal starts at zero. Input: n, then n next indices. Output: reachable cycle entry or -1.','go',2000,64,'medium','hot20',13),
('tree-level-traversal','Tree Level Traversal','Traverse a level-order encoded tree by levels. Input: token count then integer/null tokens. Output: level count then one value line per level.','go',2000,128,'medium','hot20',14),
('search-tree-validation','Search Tree Validation','Validate a strict BST from level-order integer/null tokens. Input: token count then tokens. Output: true or false; duplicates are invalid.','go',2000,128,'medium','hot20',15),
('archipelago-count','Archipelago Count','Count four-directionally connected groups of 1 cells. Input: rows cols then binary rows. Output: component count.','go',2000,128,'medium','hot20',16),
('course-dependency-order','Course Dependency Order','Output the lexicographically smallest topological order. Each pair is course prerequisite. Input: n m then pairs. Output: order or -1 on a cycle.','go',2500,128,'medium','hot20',17),
('unique-target-combinations','Unique Target Combinations','Candidates are distinct positive values reusable without limit. Output combination count then non-decreasing combinations in lexicographic order.','go',2500,128,'medium','hot20',18),
('maximum-non-adjacent-sum','Maximum Non-Adjacent Sum','Maximize the sum of non-adjacent non-negative positions. Input: n, then n values. Output: maximum sum.','go',2000,128,'medium','hot20',19),
('minimum-coin-count','Minimum Coin Count','Find the fewest reusable coins totaling amount. Input: n amount, then positive coins. Output: minimum count or -1.','go',2000,128,'medium','hot20',20)
ON CONFLICT (id) DO UPDATE SET title=EXCLUDED.title, description=EXCLUDED.description,
language=EXCLUDED.language, time_limit_ms=EXCLUDED.time_limit_ms, memory_limit_mb=EXCLUDED.memory_limit_mb,
difficulty=EXCLUDED.difficulty, collection=EXCLUDED.collection, sort_order=EXCLUDED.sort_order;

DELETE FROM problem_tags WHERE problem_id IN (SELECT id FROM problems WHERE collection IN ('starter','hot20'));
INSERT INTO problem_tags (problem_id, tag) VALUES
('sum','starter'),('echo','starter'),
('target-pair','array'),('target-pair','hash-table'),('consecutive-streak','array'),('consecutive-streak','hash-set'),
('product-without-self','array'),('product-without-self','prefix-suffix'),('widest-water-container','array'),('widest-water-container','two-pointers'),
('zero-sum-triples','array'),('zero-sum-triples','sorting'),('zero-sum-triples','two-pointers'),
('unique-character-window','string'),('unique-character-window','sliding-window'),('smallest-covering-segment','string'),('smallest-covering-segment','hash-table'),('smallest-covering-segment','sliding-window'),
('balanced-delimiters','string'),('balanced-delimiters','stack'),('warmer-day-distance','array'),('warmer-day-distance','monotonic-stack'),
('largest-skyline-rectangle','array'),('largest-skyline-rectangle','monotonic-stack'),('reverse-node-chain','linked-list'),('reverse-node-chain','iteration'),
('merge-sorted-chains','linked-list'),('merge-sorted-chains','two-pointers'),('cycle-entry','linked-list'),('cycle-entry','two-pointers'),
('tree-level-traversal','binary-tree'),('tree-level-traversal','breadth-first-search'),('search-tree-validation','binary-tree'),('search-tree-validation','depth-first-search'),('search-tree-validation','binary-search-tree'),
('archipelago-count','matrix'),('archipelago-count','depth-first-search'),('archipelago-count','breadth-first-search'),
('course-dependency-order','graph'),('course-dependency-order','topological-sort'),('course-dependency-order','heap'),
('unique-target-combinations','array'),('unique-target-combinations','backtracking'),('maximum-non-adjacent-sum','array'),('maximum-non-adjacent-sum','dynamic-programming'),
('minimum-coin-count','array'),('minimum-coin-count','dynamic-programming');

DELETE FROM problem_test_cases WHERE problem_id IN (SELECT id FROM problems WHERE collection='hot20');
INSERT INTO problem_test_cases (problem_id,input,expected_output) VALUES
('target-pair',E'4 9\n2 7 11 15\n',E'0 1\n'),('target-pair',E'3 6\n3 3 4\n',E'0 1\n'),('target-pair',E'4 10\n1 2 3 4\n',E'-1 -1\n'),('target-pair',E'5 0\n-1 1 0 0 2\n',E'0 1\n'),('target-pair',E'5 8\n4 4 4 4 4\n',E'0 1\n'),('target-pair',E'2 3\n1 2\n',E'0 1\n'),
('consecutive-streak',E'6\n100 4 200 1 3 2\n',E'4\n'),('consecutive-streak',E'0\n\n',E'0\n'),('consecutive-streak',E'7\n0 3 7 2 5 8 4\n',E'4\n'),('consecutive-streak',E'5\n1 2 2 3 3\n',E'3\n'),('consecutive-streak',E'4\n-2 -1 0 1\n',E'4\n'),('consecutive-streak',E'3\n9 7 5\n',E'1\n'),
('product-without-self',E'4\n1 2 3 4\n',E'24 12 8 6\n'),('product-without-self',E'3\n0 2 3\n',E'6 0 0\n'),('product-without-self',E'3\n0 0 3\n',E'0 0 0\n'),('product-without-self',E'1\n5\n',E'1\n'),('product-without-self',E'4\n-1 1 -1 1\n',E'-1 1 -1 1\n'),('product-without-self',E'2\n100000 100000\n',E'100000 100000\n'),
('widest-water-container',E'9\n1 8 6 2 5 4 8 3 7\n',E'49\n'),('widest-water-container',E'2\n1 1\n',E'1\n'),('widest-water-container',E'0\n\n',E'0\n'),('widest-water-container',E'4\n0 0 0 0\n',E'0\n'),('widest-water-container',E'5\n5 4 3 2 1\n',E'6\n'),('widest-water-container',E'3\n1000000000 1 1000000000\n',E'2000000000\n'),
('zero-sum-triples',E'6\n-1 0 1 2 -1 -4\n',E'2\n-1 -1 2\n-1 0 1\n'),('zero-sum-triples',E'3\n0 0 0\n',E'1\n0 0 0\n'),('zero-sum-triples',E'2\n-1 1\n',E'0\n'),('zero-sum-triples',E'5\n1 2 3 4 5\n',E'0\n'),('zero-sum-triples',E'7\n-2 0 1 1 2 -1 -4\n',E'3\n-2 0 2\n-2 1 1\n-1 0 1\n'),('zero-sum-triples',E'4\n-1 -1 2 2\n',E'1\n-1 -1 2\n'),
('unique-character-window',E'abcabcbb\n',E'3\n'),('unique-character-window',E'bbbbb\n',E'1\n'),('unique-character-window',E'\n',E'0\n'),('unique-character-window',E'pwwkew\n',E'3\n'),('unique-character-window',E'abcdef\n',E'6\n'),('unique-character-window',E'ab ba\n',E'3\n'),
('smallest-covering-segment',E'ADOBECODEBANC\nABC\n',E'BANC\n'),('smallest-covering-segment',E'a\naa\n',E'-1\n'),('smallest-covering-segment',E'abc\n\n',E'\n'),('smallest-covering-segment',E'aa\naa\n',E'aa\n'),('smallest-covering-segment',E'abdcab\nab\n',E'ab\n'),('smallest-covering-segment',E'xyyzyzyx\nxyz\n',E'zyx\n'),
('balanced-delimiters',E'()[]{}\n',E'true\n'),('balanced-delimiters',E'(]\n',E'false\n'),('balanced-delimiters',E'\n',E'true\n'),('balanced-delimiters',E'([{}])\n',E'true\n'),('balanced-delimiters',E'((\n',E'false\n'),('balanced-delimiters',E'{[}]\n',E'false\n'),
('warmer-day-distance',E'8\n73 74 75 71 69 72 76 73\n',E'1 1 4 2 1 1 0 0\n'),('warmer-day-distance',E'3\n30 40 50\n',E'1 1 0\n'),('warmer-day-distance',E'3\n50 40 30\n',E'0 0 0\n'),('warmer-day-distance',E'0\n\n',E'\n'),('warmer-day-distance',E'4\n30 30 31 30\n',E'2 1 0 0\n'),('warmer-day-distance',E'2\n-10 -5\n',E'1 0\n'),
('largest-skyline-rectangle',E'6\n2 1 5 6 2 3\n',E'10\n'),('largest-skyline-rectangle',E'2\n2 4\n',E'4\n'),('largest-skyline-rectangle',E'0\n\n',E'0\n'),('largest-skyline-rectangle',E'4\n0 0 0 0\n',E'0\n'),('largest-skyline-rectangle',E'5\n5 4 3 2 1\n',E'9\n'),('largest-skyline-rectangle',E'3\n1000000000 1000000000 1000000000\n',E'3000000000\n'),
('reverse-node-chain',E'5\n1 2 3 4 5\n',E'5 4 3 2 1\n'),('reverse-node-chain',E'0\n\n',E'\n'),('reverse-node-chain',E'1\n9\n',E'9\n'),('reverse-node-chain',E'3\n-1 0 1\n',E'1 0 -1\n'),('reverse-node-chain',E'4\n2 2 2 2\n',E'2 2 2 2\n'),('reverse-node-chain',E'2\n100 -100\n',E'-100 100\n'),
('merge-sorted-chains',E'3 3\n1 2 4\n1 3 4\n',E'1 1 2 3 4 4\n'),('merge-sorted-chains',E'0 0\n\n\n',E'\n'),('merge-sorted-chains',E'0 2\n\n1 2\n',E'1 2\n'),('merge-sorted-chains',E'2 0\n-2 3\n\n',E'-2 3\n'),('merge-sorted-chains',E'3 3\n1 1 1\n1 1 1\n',E'1 1 1 1 1 1\n'),('merge-sorted-chains',E'2 2\n-5 10\n-6 11\n',E'-6 -5 10 11\n'),
('cycle-entry',E'4\n1 2 3 1\n',E'1\n'),('cycle-entry',E'3\n1 2 -1\n',E'-1\n'),('cycle-entry',E'1\n0\n',E'0\n'),('cycle-entry',E'0\n\n',E'-1\n'),('cycle-entry',E'5\n2 -1 3 4 2\n',E'2\n'),('cycle-entry',E'4\n-1 2 3 1\n',E'-1\n'),
('tree-level-traversal',E'7\n3 9 20 null null 15 7\n',E'3\n3\n9 20\n15 7\n'),('tree-level-traversal',E'0\n\n',E'0\n'),('tree-level-traversal',E'1\n1\n',E'1\n1\n'),('tree-level-traversal',E'5\n1 2 3 4 5\n',E'3\n1\n2 3\n4 5\n'),('tree-level-traversal',E'4\n1 null 2 3\n',E'3\n1\n2\n3\n'),('tree-level-traversal',E'3\n-1 -2 -3\n',E'2\n-1\n-2 -3\n'),
('search-tree-validation',E'3\n2 1 3\n',E'true\n'),('search-tree-validation',E'7\n5 1 4 null null 3 6\n',E'false\n'),('search-tree-validation',E'0\n\n',E'true\n'),('search-tree-validation',E'1\n1\n',E'true\n'),('search-tree-validation',E'3\n2 2 3\n',E'false\n'),('search-tree-validation',E'5\n10 5 15 null 11\n',E'false\n'),
('archipelago-count',E'4 5\n11110\n11010\n11000\n00000\n',E'1\n'),('archipelago-count',E'4 5\n11000\n11000\n00100\n00011\n',E'3\n'),('archipelago-count',E'0 0\n',E'0\n'),('archipelago-count',E'1 1\n0\n',E'0\n'),('archipelago-count',E'1 1\n1\n',E'1\n'),('archipelago-count',E'3 3\n101\n010\n101\n',E'5\n'),
('course-dependency-order',E'2 1\n1 0\n',E'0 1\n'),('course-dependency-order',E'2 2\n1 0\n0 1\n',E'-1\n'),('course-dependency-order',E'4 4\n1 0\n2 0\n3 1\n3 2\n',E'0 1 2 3\n'),('course-dependency-order',E'3 0\n',E'0 1 2\n'),('course-dependency-order',E'1 0\n',E'0\n'),('course-dependency-order',E'5 4\n2 0\n2 1\n3 1\n4 2\n',E'0 1 2 3 4\n'),
('unique-target-combinations',E'4 7\n2 3 6 7\n',E'2\n2 2 3\n7\n'),('unique-target-combinations',E'3 8\n2 3 5\n',E'3\n2 2 2 2\n2 3 3\n3 5\n'),('unique-target-combinations',E'2 1\n2 3\n',E'0\n'),('unique-target-combinations',E'1 0\n5\n',E'1\n\n'),('unique-target-combinations',E'2 4\n2 4\n',E'2\n2 2\n4\n'),('unique-target-combinations',E'3 6\n1 2 3\n',E'7\n1 1 1 1 1 1\n1 1 1 1 2\n1 1 1 3\n1 1 2 2\n1 2 3\n2 2 2\n3 3\n'),
('maximum-non-adjacent-sum',E'4\n1 2 3 1\n',E'4\n'),('maximum-non-adjacent-sum',E'5\n2 7 9 3 1\n',E'12\n'),('maximum-non-adjacent-sum',E'0\n\n',E'0\n'),('maximum-non-adjacent-sum',E'1\n9\n',E'9\n'),('maximum-non-adjacent-sum',E'4\n0 0 0 0\n',E'0\n'),('maximum-non-adjacent-sum',E'6\n10 1 1 10 1 10\n',E'30\n'),
('minimum-coin-count',E'3 11\n1 2 5\n',E'3\n'),('minimum-coin-count',E'1 3\n2\n',E'-1\n'),('minimum-coin-count',E'2 0\n2 3\n',E'0\n'),('minimum-coin-count',E'3 6\n1 3 4\n',E'2\n'),('minimum-coin-count',E'2 7\n2 4\n',E'-1\n'),('minimum-coin-count',E'4 27\n2 5 10 20\n',E'3\n');
