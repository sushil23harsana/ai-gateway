-- 0001_init: virtual API keys and the append-only request log.

-- gen_random_uuid() is built into Postgres 13+; pgcrypto kept for portability.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS api_keys (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name               text        NOT NULL,
    key_hash           text        NOT NULL UNIQUE, -- sha256 of the virtual key; never store raw
    monthly_budget_usd numeric(12, 2),
    rate_limit_rpm     int         NOT NULL DEFAULT 60, -- drives the token bucket
    created_at         timestamptz NOT NULL DEFAULT now(),
    disabled           bool        NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS request_logs (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz    NOT NULL DEFAULT now(),
    api_key_id uuid REFERENCES api_keys (id),
    provider   text           NOT NULL,            -- openai | anthropic
    model      text           NOT NULL,
    status     int            NOT NULL,            -- http status returned to client
    cache_hit  bool           NOT NULL DEFAULT false,
    tokens_in  int            NOT NULL DEFAULT 0,
    tokens_out int            NOT NULL DEFAULT 0,
    cost_usd   numeric(12, 6) NOT NULL DEFAULT 0,
    latency_ms int            NOT NULL DEFAULT 0,
    error      text
);

-- Indexes for the dashboard's time-bucketed and grouped aggregates.
CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs (created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_api_key_id ON request_logs (api_key_id);
CREATE INDEX IF NOT EXISTS idx_request_logs_model ON request_logs (model);
