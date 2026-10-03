ALTER TABLE postgres_instancias DROP CONSTRAINT IF EXISTS chk_instancia_motor;

ALTER TABLE postgres_instancias ADD CONSTRAINT chk_instancia_motor
    CHECK (motor IN ('postgres', 'mysql', 'mariadb'));
