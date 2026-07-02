-- +goose Up
CREATE TABLE snapshot_checkpoints (
    id            uuid PRIMARY KEY,
    venue         text NOT NULL,
    account_type  text NOT NULL,
    taken_at      timestamptz NOT NULL,
    balance_count int NOT NULL,
    status        text NOT NULL,
    error         text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX snapshot_checkpoints_account_idx
    ON snapshot_checkpoints (venue, account_type, taken_at DESC);

-- +goose Down
DROP TABLE snapshot_checkpoints;
