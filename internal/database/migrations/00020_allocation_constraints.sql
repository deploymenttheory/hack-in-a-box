-- +goose Up
ALTER TABLE hack_teams ADD COLUMN allocation_locked boolean NOT NULL DEFAULT false;
ALTER TABLE hack_teams ADD COLUMN capacity integer CHECK(capacity BETWEEN 3 AND 10000);
UPDATE hack_teams SET allocation_locked=true WHERE state='published';

-- +goose Down
ALTER TABLE hack_teams DROP COLUMN capacity;
ALTER TABLE hack_teams DROP COLUMN allocation_locked;
