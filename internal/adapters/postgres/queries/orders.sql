-- name: InsertPendingOrder :execrows
INSERT INTO orders (client_order_id, venue, base, quote, venue_symbol, side, type, price, qty, bot_id, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending')
ON CONFLICT (client_order_id) DO NOTHING;

-- name: GetOrder :one
SELECT * FROM orders WHERE client_order_id = $1;

-- name: ListActiveOrders :many
SELECT * FROM orders
WHERE venue = $1 AND status IN ('pending', 'open', 'partially_filled')
ORDER BY created_at;

-- name: MarkCancelRequested :execrows
UPDATE orders
SET cancel_requested_at = COALESCE(cancel_requested_at, $2),
    updated_at          = now()
WHERE client_order_id = $1;

-- name: GetOrderForUpdate :one
SELECT * FROM orders WHERE client_order_id = $1 FOR UPDATE;

-- name: AdoptVenueOrderID :execrows
UPDATE orders
SET venue_order_id = $2,
    updated_at     = now()
WHERE client_order_id = $1 AND venue_order_id IS NULL;

-- name: ApplyOrderUpdate :exec
UPDATE orders
SET status         = $2,
    filled_qty     = $3,
    avg_fill_price = $4,
    venue_order_id = COALESCE(venue_order_id, $5),
    reason         = COALESCE($6, reason),
    updated_at     = now()
WHERE client_order_id = $1;

-- name: InsertTransition :one
INSERT INTO order_transitions (client_order_id, seq, from_status, to_status, filled_qty, source, reason, occurred_at)
VALUES (
    $1,
    (SELECT COALESCE(MAX(seq), 0) + 1 FROM order_transitions WHERE client_order_id = $1),
    $2, $3, $4, $5, $6, $7
)
RETURNING id, seq;

-- name: InsertFill :one
INSERT INTO fills (client_order_id, transition_id, qty, price, fee, fee_currency, venue_fill_id, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (client_order_id, venue_fill_id) WHERE venue_fill_id IS NOT NULL DO NOTHING
RETURNING id;
