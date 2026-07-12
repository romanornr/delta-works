-- +goose Up
CREATE INDEX orders_created_id_idx ON orders (created_at DESC, client_order_id DESC);

-- +goose Down
DROP INDEX orders_created_id_idx;
