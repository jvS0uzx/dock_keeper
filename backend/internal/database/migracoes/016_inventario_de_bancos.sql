ALTER TABLE servers ADD COLUMN IF NOT EXISTS collect_postgres boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS postgres_instancias (
    id bigserial PRIMARY KEY,
    server_id uuid NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    porta integer NOT NULL,
    em_container boolean NOT NULL DEFAULT false,
    container_nome character varying(128) NOT NULL DEFAULT '',
    versao character varying(32) NOT NULL DEFAULT '',
    papel character varying(16) NOT NULL DEFAULT 'desconhecido',
    wal_level character varying(16) NOT NULL DEFAULT '',
    max_wal_senders integer,
    archive_mode character varying(16) NOT NULL DEFAULT '',
    estado character varying(16) NOT NULL DEFAULT 'desconhecido',
    motivo text NOT NULL DEFAULT '',
    observado_em timestamp with time zone NOT NULL DEFAULT now(),
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_instancia_dono_porta ON postgres_instancias (server_id, porta);
CREATE INDEX IF NOT EXISTS idx_instancia_papel ON postgres_instancias (papel);

ALTER TABLE postgres_instancias ADD CONSTRAINT chk_instancia_papel
    CHECK (papel IN ('desconhecido', 'primario', 'replica'));

ALTER TABLE postgres_instancias ADD CONSTRAINT chk_instancia_estado
    CHECK (estado IN ('desconhecido', 'ativo', 'inativo', 'sem_acesso'));

CREATE TABLE IF NOT EXISTS postgres_bases (
    id bigserial PRIMARY KEY,
    instancia_id bigint NOT NULL REFERENCES postgres_instancias (id) ON DELETE CASCADE,
    nome character varying(128) NOT NULL,
    dono character varying(128) NOT NULL DEFAULT '',
    encoding character varying(32) NOT NULL DEFAULT '',
    tamanho_bytes bigint,
    conexoes integer,
    observado_em timestamp with time zone NOT NULL DEFAULT now(),
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_base_dona_nome ON postgres_bases (instancia_id, nome);
