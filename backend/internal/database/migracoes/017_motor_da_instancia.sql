ALTER TABLE postgres_instancias ADD COLUMN IF NOT EXISTS motor character varying(24) NOT NULL DEFAULT 'postgres';

CREATE INDEX IF NOT EXISTS idx_instancia_motor ON postgres_instancias (motor);

ALTER TABLE postgres_instancias ADD CONSTRAINT chk_instancia_motor
    CHECK (motor IN ('postgres'));
