CREATE TABLE IF NOT EXISTS ping_log (
  id           BIGSERIAL PRIMARY KEY,
  msg          TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
