DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
         WHERE table_schema = 'public' AND table_name = 'servers' AND column_name = 'collect_postgres'
    ) THEN
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
             WHERE table_schema = 'public' AND table_name = 'servers' AND column_name = 'collect_bancos'
        ) THEN
            ALTER TABLE servers DROP COLUMN collect_postgres;
        ELSE
            ALTER TABLE servers RENAME COLUMN collect_postgres TO collect_bancos;
        END IF;
    END IF;
END $$;

ALTER TABLE servers ADD COLUMN IF NOT EXISTS collect_bancos boolean NOT NULL DEFAULT true;

ALTER TABLE servers ALTER COLUMN collect_bancos SET DEFAULT true;

UPDATE servers SET collect_bancos = true;

ALTER TABLE servers ALTER COLUMN collect_bancos SET NOT NULL;
