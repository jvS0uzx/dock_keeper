ALTER TABLE servers ADD COLUMN IF NOT EXISTS absence_alert boolean NOT NULL DEFAULT false;
