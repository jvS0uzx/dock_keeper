ALTER TABLE alerts ADD COLUMN IF NOT EXISTS renotify_count bigint NOT NULL DEFAULT 0;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS last_notified_at timestamp with time zone;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS last_seen_at timestamp with time zone;

UPDATE alerts SET last_notified_at = last_attempt_at
WHERE last_notified_at IS NULL AND delivery = 'enviado';

UPDATE alerts SET last_seen_at = COALESCE(last_attempt_at, created_at)
WHERE last_seen_at IS NULL;

ALTER TABLE alerts DROP CONSTRAINT IF EXISTS ck_alerts_delivery;

ALTER TABLE alerts
    ADD CONSTRAINT ck_alerts_delivery CHECK (delivery IN ('pendente', 'enviado', 'falhou', 'sem_canal', 'dispensado'));

CREATE INDEX IF NOT EXISTS idx_alerts_key_status ON alerts (key, status);

ALTER TABLE device_credentials ADD COLUMN IF NOT EXISTS report_interval_sec bigint NOT NULL DEFAULT 0;
