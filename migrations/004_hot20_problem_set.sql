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
