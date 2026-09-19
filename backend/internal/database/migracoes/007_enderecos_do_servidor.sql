CREATE TABLE IF NOT EXISTS server_addresses (
    id bigserial PRIMARY KEY,
    server_id uuid NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    address varchar(45) NOT NULL,
    origem varchar(16) NOT NULL DEFAULT 'coletado',
    primeiro_visto timestamptz NOT NULL DEFAULT now(),
    ultimo_visto timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_server_addresses_origem CHECK (origem IN ('coletado', 'manual'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_server_addresses_srv_addr ON server_addresses (server_id, address);
CREATE INDEX IF NOT EXISTS idx_server_addresses_address ON server_addresses (address);
