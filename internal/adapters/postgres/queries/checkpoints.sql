-- name: RecordSnapshot :exec
INSERT INTO snapshot_checkpoints (id, venue, account_type, taken_at, balance_count, status, error)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: LastSnapshot :one
SELECT id, venue, account_type, taken_at, balance_count, status, error, created_at
FROM snapshot_checkpoints
WHERE venue = $1 AND account_type = $2
ORDER BY taken_at DESC, created_at DESC
LIMIT 1;
