-- Safe cleanup: only deletes submissions created by k6 load tests.
-- Does NOT flush Redis, trim streams, or delete ordinary submissions.
DELETE FROM submissions WHERE code LIKE '%k6-loadtest%';
