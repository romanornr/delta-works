-- +goose Up
CREATE TABLE lots (
    id                text PRIMARY KEY,
    bot_id            text NOT NULL,
    venue             text NOT NULL,
    base              text NOT NULL,
    quote             text NOT NULL,
    qty               numeric NOT NULL CHECK (qty > 0),
    remaining_qty     numeric NOT NULL CHECK (remaining_qty >= 0 AND remaining_qty <= qty),
    cost_price        numeric NOT NULL CHECK (cost_price >= 0),
    opened_by_fill_id bigint NOT NULL UNIQUE REFERENCES fills (id),
    status            text NOT NULL CHECK (status IN ('open', 'closed')),
    opened_at         timestamptz NOT NULL,
    closed_at         timestamptz,
    CHECK ((status = 'open') = (closed_at IS NULL)),
    CHECK (status = 'closed' OR remaining_qty > 0),
    CHECK (status = 'open' OR remaining_qty = 0)
);

CREATE INDEX lots_open_fifo_idx
    ON lots (bot_id, venue, base, quote, opened_at, id)
    WHERE status = 'open';

CREATE TABLE lot_closures (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    lot_id       text NOT NULL REFERENCES lots (id),
    sell_fill_id bigint NOT NULL REFERENCES fills (id),
    qty          numeric NOT NULL CHECK (qty > 0),
    price        numeric NOT NULL CHECK (price >= 0),
    closed_at    timestamptz NOT NULL,
    UNIQUE (lot_id, sell_fill_id)
);

CREATE TABLE unmatched_sells (
    sell_fill_id bigint PRIMARY KEY REFERENCES fills (id),
    bot_id       text NOT NULL,
    venue        text NOT NULL,
    base         text NOT NULL,
    quote        text NOT NULL,
    qty          numeric NOT NULL CHECK (qty > 0),
    occurred_at  timestamptz NOT NULL
);

-- +goose Down
DROP TABLE unmatched_sells;
DROP TABLE lot_closures;
DROP TABLE lots;
