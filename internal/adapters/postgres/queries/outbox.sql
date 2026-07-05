-- name: InsertOutbox :exec
INSERT INTO outbox (subject, payload) VALUES ($1, $2);

-- name: ClaimUnpublishedOutbox :many
SELECT id, subject, payload, created_at FROM outbox
WHERE published_at IS NULL
ORDER BY id
LIMIT $1::bigint
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxPublished :exec
UPDATE outbox SET published_at = now() WHERE id = ANY($1::bigint[]);

-- name: DeleteOutboxPublishedBefore :execrows
DELETE FROM outbox WHERE published_at IS NOT NULL AND published_at < $1;

-- name: OutboxUnpublishedStats :one
SELECT COUNT(*) AS unpublished, COALESCE(MIN(created_at), now())::timestamptz AS oldest
FROM outbox
WHERE published_at IS NULL;
