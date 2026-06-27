INSERT INTO problems (id, title, description, language, time_limit_ms, memory_limit_mb)
VALUES
    ('sum', 'A+B Problem', 'Read two integers from standard input and print their sum.', 'go', 2000, 64),
    ('echo', 'Echo', 'Read one line and print it unchanged.', 'go', 2000, 64)
ON CONFLICT (id) DO NOTHING;

INSERT INTO problem_test_cases (problem_id, input, expected_output)
VALUES
    ('sum', E'1 2\n', E'3\n'),
    ('sum', E'10 32\n', E'42\n'),
    ('echo', E'hello\n', E'hello\n');
