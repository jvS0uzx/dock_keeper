ALTER TABLE network_hosts ADD COLUMN IF NOT EXISTS snmp_sys_name character varying(255) NOT NULL DEFAULT '';
ALTER TABLE network_hosts ADD COLUMN IF NOT EXISTS snmp_sys_descr character varying(255) NOT NULL DEFAULT '';
ALTER TABLE network_hosts ADD COLUMN IF NOT EXISTS snmp_uptime_sec bigint;
ALTER TABLE network_hosts ADD COLUMN IF NOT EXISTS snmp_visto_em timestamp with time zone;
ALTER TABLE network_hosts ADD COLUMN IF NOT EXISTS snmp_erro text NOT NULL DEFAULT '';
ALTER TABLE network_hosts ADD COLUMN IF NOT EXISTS snmp_erro_em timestamp with time zone;

CREATE TABLE IF NOT EXISTS network_interfaces (
    id bigserial PRIMARY KEY,
    network_host_id bigint NOT NULL REFERENCES network_hosts (id) ON DELETE CASCADE,
    if_index integer NOT NULL,
    if_name character varying(128) NOT NULL DEFAULT '',
    if_descr character varying(128) NOT NULL DEFAULT '',
    if_alias character varying(128) NOT NULL DEFAULT '',
    speed_mbps bigint,
    oper_status character varying(16) NOT NULL DEFAULT 'unknown',
    admin_status character varying(16) NOT NULL DEFAULT 'unknown',
    first_seen timestamp with time zone NOT NULL DEFAULT now(),
    last_seen timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_interface_host_nome
    ON network_interfaces (network_host_id, if_name) WHERE if_name <> '';
CREATE INDEX IF NOT EXISTS idx_interface_host_indice ON network_interfaces (network_host_id, if_index);

ALTER TABLE network_interfaces DROP CONSTRAINT IF EXISTS ck_interface_oper_status;
ALTER TABLE network_interfaces ADD CONSTRAINT ck_interface_oper_status
    CHECK (oper_status IN ('up', 'down', 'testing', 'unknown', 'dormant', 'notPresent', 'lowerLayerDown'));
ALTER TABLE network_interfaces DROP CONSTRAINT IF EXISTS ck_interface_admin_status;
ALTER TABLE network_interfaces ADD CONSTRAINT ck_interface_admin_status
    CHECK (admin_status IN ('up', 'down', 'testing', 'unknown', 'dormant', 'notPresent', 'lowerLayerDown'));

CREATE TABLE IF NOT EXISTS metric_network_interfaces (
    id bigserial PRIMARY KEY,
    interface_id bigint NOT NULL REFERENCES network_interfaces (id) ON DELETE CASCADE,
    ts timestamp with time zone NOT NULL,
    in_bps double precision,
    out_bps double precision,
    in_errors bigint,
    out_errors bigint,
    in_discards bigint,
    out_discards bigint,
    oper_status character varying(16) NOT NULL DEFAULT 'unknown'
);

CREATE INDEX IF NOT EXISTS idx_metric_interface_ts ON metric_network_interfaces (interface_id, ts);
CREATE INDEX IF NOT EXISTS idx_metric_interface_poda ON metric_network_interfaces (ts);

ALTER TABLE metric_network_interfaces DROP CONSTRAINT IF EXISTS ck_metric_interface_oper_status;
ALTER TABLE metric_network_interfaces ADD CONSTRAINT ck_metric_interface_oper_status
    CHECK (oper_status IN ('up', 'down', 'testing', 'unknown', 'dormant', 'notPresent', 'lowerLayerDown'));
