DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
EXCEPTION
    WHEN insufficient_privilege OR feature_not_supported THEN
        RAISE WARNING 'pg_trgm não pôde ser criada (%); a busca de log segue sem índice de trigrama', SQLERRM;
END $$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        CREATE INDEX IF NOT EXISTS idx_log_entries_line_trgm
            ON log_entries USING gin (line gin_trgm_ops);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_log_entries_timestamp ON log_entries ("timestamp" DESC);
