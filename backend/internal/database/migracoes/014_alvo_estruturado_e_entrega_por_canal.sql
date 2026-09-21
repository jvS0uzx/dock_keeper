ALTER TABLE alerts ADD COLUMN IF NOT EXISTS alvo_tipo character varying(16);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS alvo_id character varying(128);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS alvo_nome character varying(255);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS metrica character varying(64);
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS valor double precision;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS limiar double precision;
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS unidade character varying(16);

CREATE INDEX IF NOT EXISTS idx_alerts_alvo_tipo ON alerts (alvo_tipo);

ALTER TABLE alerts ADD CONSTRAINT chk_alerts_alvo_tipo
    CHECK (alvo_tipo IS NULL OR alvo_tipo IN ('host', 'container', 'servico', 'interface'));

CREATE TABLE IF NOT EXISTS alert_deliveries (
    id bigserial PRIMARY KEY,
    alert_id bigint NOT NULL REFERENCES alerts (id) ON DELETE CASCADE,
    canal character varying(32) NOT NULL,
    status character varying(16) NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamp with time zone,
    last_attempt_at timestamp with time zone,
    last_error text NOT NULL DEFAULT '',
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_entrega_alerta_canal ON alert_deliveries (alert_id, canal);
CREATE INDEX IF NOT EXISTS idx_alert_deliveries_status ON alert_deliveries (status);
CREATE INDEX IF NOT EXISTS idx_alert_deliveries_next_attempt_at ON alert_deliveries (next_attempt_at);

ALTER TABLE alert_deliveries ADD CONSTRAINT chk_alert_deliveries_status
    CHECK (status IN ('pendente', 'enviado', 'falhou', 'sem_canal', 'dispensado'));

INSERT INTO alert_deliveries (alert_id, canal, status, attempts, next_attempt_at, last_attempt_at, last_error)
SELECT id, 'telegram', delivery, attempts, next_attempt_at, last_attempt_at, COALESCE(last_error, '')
FROM alerts
ON CONFLICT (alert_id, canal) DO NOTHING;
