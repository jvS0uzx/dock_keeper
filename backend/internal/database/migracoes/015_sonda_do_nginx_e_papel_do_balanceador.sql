ALTER TABLE servers ADD COLUMN IF NOT EXISTS nginx_estado character varying(16) NOT NULL DEFAULT 'desconhecido';
ALTER TABLE servers ADD COLUMN IF NOT EXISTS nginx_motivo text NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN IF NOT EXISTS nginx_checado_em timestamp with time zone;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS nginx_papel character varying(16) NOT NULL DEFAULT 'nenhum';
ALTER TABLE servers ADD COLUMN IF NOT EXISTS nginx_papel_desde timestamp with time zone;

CREATE INDEX IF NOT EXISTS idx_servers_nginx_estado ON servers (nginx_estado);
CREATE INDEX IF NOT EXISTS idx_servers_nginx_papel ON servers (nginx_papel);

ALTER TABLE servers ADD CONSTRAINT chk_servers_nginx_estado
    CHECK (nginx_estado IN ('desconhecido', 'ausente', 'inativo', 'sem_upstream', 'candidato'));

ALTER TABLE servers ADD CONSTRAINT chk_servers_nginx_papel
    CHECK (nginx_papel IN ('nenhum', 'principal', 'reserva'));

CREATE TABLE IF NOT EXISTS nginx_upstreams (
    id bigserial PRIMARY KEY,
    server_id uuid NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    bloco character varying(128) NOT NULL,
    destino character varying(255) NOT NULL,
    observado_em timestamp with time zone NOT NULL DEFAULT now(),
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_dono_bloco_destino ON nginx_upstreams (server_id, bloco, destino);
CREATE INDEX IF NOT EXISTS idx_nginx_upstreams_server_id ON nginx_upstreams (server_id);
CREATE INDEX IF NOT EXISTS idx_nginx_upstreams_observado_em ON nginx_upstreams (observado_em);

UPDATE servers SET nginx_estado = 'desconhecido' WHERE nginx_estado IS NULL;
