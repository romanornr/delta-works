-- +goose Up
CREATE TABLE orders (
    client_order_id     text PRIMARY KEY,
    venue               text NOT NULL,
    base                text NOT NULL,
    quote               text NOT NULL,
    venue_symbol        text NOT NULL,
    side                text NOT NULL CHECK (side IN ('buy', 'sell')),
    type                text NOT NULL CHECK (type IN ('limit', 'market')),
    price               numeric NOT NULL,
    qty                 numeric NOT NULL,
    filled_qty          numeric NOT NULL DEFAULT 0,
    avg_fill_price      numeric,
    status              text NOT NULL CHECK (status IN ('pending', 'open', 'partially_filled', 'filled', 'canceled', 'rejected', 'expired')),
    venue_order_id      text,
    bot_id              text NOT NULL,
    cancel_requested_at timestamptz,
    reason              text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX orders_venue_order_id_key ON orders (venue, venue_order_id) WHERE venue_order_id IS NOT NULL;
CREATE INDEX orders_venue_status_idx ON orders (venue, status);
CREATE INDEX orders_bot_created_idx ON orders (bot_id, created_at DESC);

CREATE TABLE order_transitions (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_order_id text NOT NULL REFERENCES orders (client_order_id),
    seq             integer NOT NULL,
    from_status     text NOT NULL,
    to_status       text NOT NULL,
    filled_qty      numeric NOT NULL,
    source          text NOT NULL CHECK (source IN ('local', 'stream', 'ack', 'reconcile')),
    reason          text,
    occurred_at     timestamptz NOT NULL,
    recorded_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (client_order_id, seq)
);

CREATE TABLE fills (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    client_order_id text NOT NULL REFERENCES orders (client_order_id),
    transition_id   bigint NOT NULL REFERENCES order_transitions (id),
    qty             numeric NOT NULL,
    price           numeric,
    fee             numeric,
    fee_currency    text,
    venue_fill_id   text,
    occurred_at     timestamptz NOT NULL
);

CREATE UNIQUE INDEX fills_venue_fill_id_key ON fills (client_order_id, venue_fill_id) WHERE venue_fill_id IS NOT NULL;

-- +goose Down
DROP TABLE fills;
DROP TABLE order_transitions;
DROP TABLE orders;
