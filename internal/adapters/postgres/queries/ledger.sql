-- name: LockInventory :exec
SELECT pg_advisory_xact_lock(sqlc.arg(key)::bigint);

-- name: InsertLot :exec
INSERT INTO lots (
    id, bot_id, venue, base, quote, qty, remaining_qty, cost_price,
    opened_by_fill_id, status, opened_at
)
VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8, 'open', $9);

-- name: ListOpenLotsForUpdate :many
SELECT * FROM lots
WHERE bot_id = $1 AND venue = $2 AND base = $3 AND quote = $4 AND status = 'open'
ORDER BY opened_at, id
FOR UPDATE;

-- name: InsertLotClosure :exec
INSERT INTO lot_closures (lot_id, sell_fill_id, qty, price, closed_at)
VALUES ($1, $2, $3, $4, $5);

-- name: DecrementLot :exec
UPDATE lots
SET remaining_qty = remaining_qty - sqlc.arg(qty),
    status = CASE WHEN remaining_qty - sqlc.arg(qty) = 0 THEN 'closed' ELSE 'open' END,
    closed_at = CASE WHEN remaining_qty - sqlc.arg(qty) = 0 THEN sqlc.arg(closed_at)::timestamptz ELSE NULL END
WHERE id = sqlc.arg(id);

-- name: InsertUnmatchedSell :exec
INSERT INTO unmatched_sells (sell_fill_id, bot_id, venue, base, quote, qty, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);
