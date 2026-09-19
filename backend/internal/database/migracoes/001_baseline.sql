CREATE TABLE IF NOT EXISTS public.alert_rules (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    target character varying(64) DEFAULT '*'::character varying NOT NULL,
    metric character varying(32) NOT NULL,
    operator character varying(4) NOT NULL,
    threshold numeric NOT NULL,
    enabled boolean,
    last_fired timestamp with time zone,
    severity character varying(16) DEFAULT 'warning'::character varying NOT NULL,
    depends_on_server_id uuid,
    target_site_id bigint,
    for_duration_sec bigint DEFAULT 0,
    created_at timestamp with time zone
);

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS name character varying(255);

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS target character varying(64);

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS metric character varying(32);

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS operator character varying(4);

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS threshold numeric;

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS enabled boolean;

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS last_fired timestamp with time zone;

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS severity character varying(16);

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS depends_on_server_id uuid;

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS target_site_id bigint;

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS for_duration_sec bigint;

ALTER TABLE public.alert_rules ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.alert_rules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.alert_rules_id_seq OWNED BY public.alert_rules.id;

CREATE TABLE IF NOT EXISTS public.alert_states (
    key character varying(128) NOT NULL,
    rule_id bigint NOT NULL,
    server_id character varying(64),
    severity character varying(16),
    first_breach_at timestamp with time zone,
    last_breach_at timestamp with time zone,
    last_notified_at timestamp with time zone,
    active boolean DEFAULT false,
    updated_at timestamp with time zone
);

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS key character varying(128);

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS rule_id bigint;

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS server_id character varying(64);

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS severity character varying(16);

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS first_breach_at timestamp with time zone;

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS last_breach_at timestamp with time zone;

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS last_notified_at timestamp with time zone;

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS active boolean;

ALTER TABLE public.alert_states ADD COLUMN IF NOT EXISTS updated_at timestamp with time zone;

CREATE TABLE IF NOT EXISTS public.alerts (
    id bigint NOT NULL,
    key character varying(128) NOT NULL,
    severity character varying(16) NOT NULL,
    text text NOT NULL,
    status character varying(16) NOT NULL,
    server_id uuid,
    site_id bigint,
    rule_id bigint,
    created_at timestamp with time zone,
    acked_at timestamp with time zone,
    acked_by bigint,
    resolved_at timestamp with time zone,
    delivery character varying(16) NOT NULL,
    attempts bigint NOT NULL,
    next_attempt_at timestamp with time zone,
    last_attempt_at timestamp with time zone,
    last_error text
);

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS key character varying(128);

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS severity character varying(16);

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS text text;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS status character varying(16);

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS rule_id bigint;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS acked_at timestamp with time zone;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS acked_by bigint;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS resolved_at timestamp with time zone;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS delivery character varying(16);

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS attempts bigint;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS next_attempt_at timestamp with time zone;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS last_attempt_at timestamp with time zone;

ALTER TABLE public.alerts ADD COLUMN IF NOT EXISTS last_error text;

CREATE SEQUENCE IF NOT EXISTS public.alerts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.alerts_id_seq OWNED BY public.alerts.id;

CREATE TABLE IF NOT EXISTS public.annotations (
    id bigint NOT NULL,
    server_id uuid,
    at timestamp with time zone NOT NULL,
    text character varying(280) NOT NULL,
    author_user_id bigint NOT NULL,
    created_at timestamp with time zone
);

ALTER TABLE public.annotations ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.annotations ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.annotations ADD COLUMN IF NOT EXISTS at timestamp with time zone;

ALTER TABLE public.annotations ADD COLUMN IF NOT EXISTS text character varying(280);

ALTER TABLE public.annotations ADD COLUMN IF NOT EXISTS author_user_id bigint;

ALTER TABLE public.annotations ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.annotations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.annotations_id_seq OWNED BY public.annotations.id;

CREATE TABLE IF NOT EXISTS public.audit_logs (
    id bigint NOT NULL,
    at timestamp with time zone NOT NULL,
    actor_user_id bigint,
    actor_username character varying(64),
    actor_role character varying(16),
    source_ip character varying(45),
    user_agent character varying(255),
    action character varying(64) NOT NULL,
    target_type character varying(32),
    target_id character varying(64),
    target_label character varying(255),
    site_id bigint,
    result character varying(16) NOT NULL,
    detail jsonb
);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS at timestamp with time zone;

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS actor_user_id bigint;

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS actor_username character varying(64);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS actor_role character varying(16);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS source_ip character varying(45);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS user_agent character varying(255);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS action character varying(64);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS target_type character varying(32);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS target_id character varying(64);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS target_label character varying(255);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS result character varying(16);

ALTER TABLE public.audit_logs ADD COLUMN IF NOT EXISTS detail jsonb;

CREATE SEQUENCE IF NOT EXISTS public.audit_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.audit_logs_id_seq OWNED BY public.audit_logs.id;

CREATE TABLE IF NOT EXISTS public.containers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    server_id uuid NOT NULL,
    docker_id character varying(64) NOT NULL,
    name character varying(255) NOT NULL,
    project_dir character varying(500),
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE public.containers ADD COLUMN IF NOT EXISTS id uuid;

ALTER TABLE public.containers ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.containers ADD COLUMN IF NOT EXISTS docker_id character varying(64);

ALTER TABLE public.containers ADD COLUMN IF NOT EXISTS name character varying(255);

ALTER TABLE public.containers ADD COLUMN IF NOT EXISTS project_dir character varying(500);

ALTER TABLE public.containers ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

ALTER TABLE public.containers ADD COLUMN IF NOT EXISTS updated_at timestamp with time zone;

CREATE TABLE IF NOT EXISTS public.dashboard_panels (
    id bigint NOT NULL,
    dashboard_id bigint NOT NULL,
    "position" bigint NOT NULL,
    title character varying(80),
    server_id uuid NOT NULL,
    metric character varying(16) NOT NULL,
    range character varying(8) NOT NULL,
    width bigint NOT NULL
);

ALTER TABLE public.dashboard_panels ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.dashboard_panels ADD COLUMN IF NOT EXISTS dashboard_id bigint;

ALTER TABLE public.dashboard_panels ADD COLUMN IF NOT EXISTS "position" bigint;

ALTER TABLE public.dashboard_panels ADD COLUMN IF NOT EXISTS title character varying(80);

ALTER TABLE public.dashboard_panels ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.dashboard_panels ADD COLUMN IF NOT EXISTS metric character varying(16);

ALTER TABLE public.dashboard_panels ADD COLUMN IF NOT EXISTS range character varying(8);

ALTER TABLE public.dashboard_panels ADD COLUMN IF NOT EXISTS width bigint;

CREATE SEQUENCE IF NOT EXISTS public.dashboard_panels_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.dashboard_panels_id_seq OWNED BY public.dashboard_panels.id;

CREATE TABLE IF NOT EXISTS public.dashboards (
    id bigint NOT NULL,
    owner_user_id bigint NOT NULL,
    name character varying(80) NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE public.dashboards ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.dashboards ADD COLUMN IF NOT EXISTS owner_user_id bigint;

ALTER TABLE public.dashboards ADD COLUMN IF NOT EXISTS name character varying(80);

ALTER TABLE public.dashboards ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

ALTER TABLE public.dashboards ADD COLUMN IF NOT EXISTS updated_at timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.dashboards_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.dashboards_id_seq OWNED BY public.dashboards.id;

CREATE TABLE IF NOT EXISTS public.device_credentials (
    device_id character varying(32) NOT NULL,
    secret_hash character varying(64) NOT NULL,
    site_id bigint NOT NULL,
    kind character varying(16) NOT NULL,
    machine_id character varying(128),
    hostname character varying(255),
    created_at timestamp with time zone,
    last_seen_at timestamp with time zone,
    revoked_at timestamp with time zone
);

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS device_id character varying(32);

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS secret_hash character varying(64);

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS kind character varying(16);

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS machine_id character varying(128);

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS hostname character varying(255);

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS last_seen_at timestamp with time zone;

ALTER TABLE public.device_credentials ADD COLUMN IF NOT EXISTS revoked_at timestamp with time zone;

CREATE TABLE IF NOT EXISTS public.domains (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    server_id uuid,
    created_at timestamp with time zone,
    valid boolean DEFAULT false,
    issuer character varying(255),
    days_left bigint DEFAULT 0,
    error_msg character varying(500),
    invalid_reason character varying(40),
    last_check timestamp with time zone
);

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS name character varying(255);

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS valid boolean;

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS issuer character varying(255);

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS days_left bigint;

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS error_msg character varying(500);

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS invalid_reason character varying(40);

ALTER TABLE public.domains ADD COLUMN IF NOT EXISTS last_check timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.domains_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.domains_id_seq OWNED BY public.domains.id;

CREATE TABLE IF NOT EXISTS public.enrollment_tokens (
    id bigint NOT NULL,
    token_hash character varying(64) NOT NULL,
    site_id bigint NOT NULL,
    kind character varying(16) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_by bigint,
    created_at timestamp with time zone
);

ALTER TABLE public.enrollment_tokens ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.enrollment_tokens ADD COLUMN IF NOT EXISTS token_hash character varying(64);

ALTER TABLE public.enrollment_tokens ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.enrollment_tokens ADD COLUMN IF NOT EXISTS kind character varying(16);

ALTER TABLE public.enrollment_tokens ADD COLUMN IF NOT EXISTS expires_at timestamp with time zone;

ALTER TABLE public.enrollment_tokens ADD COLUMN IF NOT EXISTS used_at timestamp with time zone;

ALTER TABLE public.enrollment_tokens ADD COLUMN IF NOT EXISTS created_by bigint;

ALTER TABLE public.enrollment_tokens ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.enrollment_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.enrollment_tokens_id_seq OWNED BY public.enrollment_tokens.id;

CREATE TABLE IF NOT EXISTS public.floor_plan_pins (
    id bigint NOT NULL,
    plan_id bigint NOT NULL,
    host_ip character varying(45),
    label character varying(255),
    x numeric,
    y numeric,
    target_plan_id bigint
);

ALTER TABLE public.floor_plan_pins ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.floor_plan_pins ADD COLUMN IF NOT EXISTS plan_id bigint;

ALTER TABLE public.floor_plan_pins ADD COLUMN IF NOT EXISTS host_ip character varying(45);

ALTER TABLE public.floor_plan_pins ADD COLUMN IF NOT EXISTS label character varying(255);

ALTER TABLE public.floor_plan_pins ADD COLUMN IF NOT EXISTS x numeric;

ALTER TABLE public.floor_plan_pins ADD COLUMN IF NOT EXISTS y numeric;

ALTER TABLE public.floor_plan_pins ADD COLUMN IF NOT EXISTS target_plan_id bigint;

CREATE SEQUENCE IF NOT EXISTS public.floor_plan_pins_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.floor_plan_pins_id_seq OWNED BY public.floor_plan_pins.id;

CREATE TABLE IF NOT EXISTS public.floor_plans (
    id bigint NOT NULL,
    site_id bigint,
    name character varying(255) NOT NULL,
    image_path character varying(500),
    content_type character varying(64),
    width bigint,
    height bigint,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS name character varying(255);

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS image_path character varying(500);

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS content_type character varying(64);

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS width bigint;

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS height bigint;

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

ALTER TABLE public.floor_plans ADD COLUMN IF NOT EXISTS updated_at timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.floor_plans_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.floor_plans_id_seq OWNED BY public.floor_plans.id;

CREATE TABLE IF NOT EXISTS public.log_entries (
    id bigint NOT NULL,
    server_id uuid,
    source character varying(20),
    container character varying(255),
    line text,
    "timestamp" timestamp with time zone
);

ALTER TABLE public.log_entries ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.log_entries ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.log_entries ADD COLUMN IF NOT EXISTS source character varying(20);

ALTER TABLE public.log_entries ADD COLUMN IF NOT EXISTS container character varying(255);

ALTER TABLE public.log_entries ADD COLUMN IF NOT EXISTS line text;

ALTER TABLE public.log_entries ADD COLUMN IF NOT EXISTS "timestamp" timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.log_entries_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.log_entries_id_seq OWNED BY public.log_entries.id;

CREATE TABLE IF NOT EXISTS public.metric_containers (
    id bigint NOT NULL,
    container_id uuid NOT NULL,
    cpu_usage_percent numeric NOT NULL,
    mem_used_bytes bigint NOT NULL,
    mem_limit_bytes bigint NOT NULL,
    state character varying(50) DEFAULT 'running'::character varying NOT NULL,
    status character varying(255) DEFAULT ''::character varying NOT NULL,
    "timestamp" timestamp with time zone NOT NULL
);

ALTER TABLE public.metric_containers ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.metric_containers ADD COLUMN IF NOT EXISTS container_id uuid;

ALTER TABLE public.metric_containers ADD COLUMN IF NOT EXISTS cpu_usage_percent numeric;

ALTER TABLE public.metric_containers ADD COLUMN IF NOT EXISTS mem_used_bytes bigint;

ALTER TABLE public.metric_containers ADD COLUMN IF NOT EXISTS mem_limit_bytes bigint;

ALTER TABLE public.metric_containers ADD COLUMN IF NOT EXISTS state character varying(50);

ALTER TABLE public.metric_containers ADD COLUMN IF NOT EXISTS status character varying(255);

ALTER TABLE public.metric_containers ADD COLUMN IF NOT EXISTS "timestamp" timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.metric_containers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.metric_containers_id_seq OWNED BY public.metric_containers.id;

CREATE TABLE IF NOT EXISTS public.metric_load_balancers (
    id bigint NOT NULL,
    upstream_addr character varying(255) NOT NULL,
    server_name character varying(255),
    status character varying(10),
    server_id uuid,
    site_id bigint,
    requests_count bigint NOT NULL,
    "timestamp" timestamp with time zone NOT NULL
);

ALTER TABLE public.metric_load_balancers ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.metric_load_balancers ADD COLUMN IF NOT EXISTS upstream_addr character varying(255);

ALTER TABLE public.metric_load_balancers ADD COLUMN IF NOT EXISTS server_name character varying(255);

ALTER TABLE public.metric_load_balancers ADD COLUMN IF NOT EXISTS status character varying(10);

ALTER TABLE public.metric_load_balancers ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.metric_load_balancers ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.metric_load_balancers ADD COLUMN IF NOT EXISTS requests_count bigint;

ALTER TABLE public.metric_load_balancers ADD COLUMN IF NOT EXISTS "timestamp" timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.metric_load_balancers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.metric_load_balancers_id_seq OWNED BY public.metric_load_balancers.id;

CREATE TABLE IF NOT EXISTS public.metric_server_trends (
    id bigint NOT NULL,
    server_id uuid NOT NULL,
    bucket timestamp with time zone NOT NULL,
    cpu_avg numeric,
    cpu_max numeric,
    load_avg1_avg numeric,
    load_avg1_max numeric,
    samples bigint,
    mem_percent_avg numeric,
    disk_percent_avg numeric,
    temperature_avg numeric,
    temperature_max numeric,
    net_rx_avg numeric,
    net_tx_avg numeric,
    rtt_avg numeric,
    rtt_max numeric
);

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS bucket timestamp with time zone;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS cpu_avg numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS cpu_max numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS load_avg1_avg numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS load_avg1_max numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS samples bigint;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS mem_percent_avg numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS disk_percent_avg numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS temperature_avg numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS temperature_max numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS net_rx_avg numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS net_tx_avg numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS rtt_avg numeric;

ALTER TABLE public.metric_server_trends ADD COLUMN IF NOT EXISTS rtt_max numeric;

CREATE SEQUENCE IF NOT EXISTS public.metric_server_trends_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.metric_server_trends_id_seq OWNED BY public.metric_server_trends.id;

CREATE TABLE IF NOT EXISTS public.metric_servers (
    id bigint NOT NULL,
    server_id uuid NOT NULL,
    uptime_seconds numeric,
    disk_used_bytes bigint,
    disk_total_bytes bigint,
    cpu_usage_percent numeric,
    mem_used_bytes bigint,
    mem_total_bytes bigint,
    load_avg1 numeric,
    ping_latency_ms numeric,
    temperature_c numeric,
    net_rx_bps numeric,
    net_tx_bps numeric,
    rtt_ms numeric,
    "timestamp" timestamp with time zone NOT NULL
);

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS server_id uuid;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS uptime_seconds numeric;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS disk_used_bytes bigint;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS disk_total_bytes bigint;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS cpu_usage_percent numeric;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS mem_used_bytes bigint;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS mem_total_bytes bigint;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS load_avg1 numeric;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS ping_latency_ms numeric;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS temperature_c numeric;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS net_rx_bps numeric;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS net_tx_bps numeric;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS rtt_ms numeric;

ALTER TABLE public.metric_servers ADD COLUMN IF NOT EXISTS "timestamp" timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.metric_servers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.metric_servers_id_seq OWNED BY public.metric_servers.id;

CREATE TABLE IF NOT EXISTS public.network_hosts (
    id bigint NOT NULL,
    ip character varying(45),
    hostname character varying(255),
    mac character varying(32),
    open_ports character varying(255),
    first_seen timestamp with time zone,
    last_seen timestamp with time zone,
    device_type character varying(64),
    device_type_locked boolean DEFAULT false,
    site_id bigint,
    site_locked boolean DEFAULT false,
    floor character varying(64),
    sector character varying(128),
    room character varying(64),
    rack character varying(64),
    asset_tag character varying(64),
    owner character varying(255),
    notes text
);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS ip character varying(45);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS hostname character varying(255);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS mac character varying(32);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS open_ports character varying(255);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS first_seen timestamp with time zone;

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS last_seen timestamp with time zone;

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS device_type character varying(64);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS device_type_locked boolean;

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS site_locked boolean;

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS floor character varying(64);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS sector character varying(128);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS room character varying(64);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS rack character varying(64);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS asset_tag character varying(64);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS owner character varying(255);

ALTER TABLE public.network_hosts ADD COLUMN IF NOT EXISTS notes text;

CREATE SEQUENCE IF NOT EXISTS public.network_hosts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.network_hosts_id_seq OWNED BY public.network_hosts.id;

CREATE TABLE IF NOT EXISTS public.servers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(255) NOT NULL,
    host_ip character varying(255) NOT NULL,
    "user" character varying(100) DEFAULT 'root'::character varying,
    port bigint DEFAULT 22,
    kind character varying(20) DEFAULT 'ssh'::character varying,
    collect_nginx boolean DEFAULT false,
    site_id bigint,
    os character varying(64),
    platform character varying(128),
    arch character varying(32),
    agent_version character varying(32),
    last_user character varying(128),
    report_interval_sec bigint DEFAULT 0,
    machine_id character varying(128),
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone
);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS id uuid;

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS name character varying(255);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS host_ip character varying(255);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS "user" character varying(100);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS port bigint;

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS kind character varying(20);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS collect_nginx boolean;

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS os character varying(64);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS platform character varying(128);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS arch character varying(32);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS agent_version character varying(32);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS last_user character varying(128);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS report_interval_sec bigint;

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS machine_id character varying(128);

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS updated_at timestamp with time zone;

ALTER TABLE public.servers ADD COLUMN IF NOT EXISTS deleted_at timestamp with time zone;

CREATE TABLE IF NOT EXISTS public.sites (
    id bigint NOT NULL,
    name character varying(255) NOT NULL,
    code character varying(64),
    address character varying(500),
    latitude numeric,
    longitude numeric,
    created_at timestamp with time zone
);

ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS name character varying(255);

ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS code character varying(64);

ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS address character varying(500);

ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS latitude numeric;

ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS longitude numeric;

ALTER TABLE public.sites ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.sites_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.sites_id_seq OWNED BY public.sites.id;

CREATE TABLE IF NOT EXISTS public.user_sessions (
    token_hash character varying(64) NOT NULL,
    user_id bigint NOT NULL,
    role character varying(16) NOT NULL,
    username character varying(64),
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone,
    last_seen_at timestamp with time zone
);

ALTER TABLE public.user_sessions ADD COLUMN IF NOT EXISTS token_hash character varying(64);

ALTER TABLE public.user_sessions ADD COLUMN IF NOT EXISTS user_id bigint;

ALTER TABLE public.user_sessions ADD COLUMN IF NOT EXISTS role character varying(16);

ALTER TABLE public.user_sessions ADD COLUMN IF NOT EXISTS username character varying(64);

ALTER TABLE public.user_sessions ADD COLUMN IF NOT EXISTS expires_at timestamp with time zone;

ALTER TABLE public.user_sessions ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

ALTER TABLE public.user_sessions ADD COLUMN IF NOT EXISTS last_seen_at timestamp with time zone;

CREATE TABLE IF NOT EXISTS public.user_site_accesses (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    site_id bigint,
    role character varying(16) NOT NULL
);

ALTER TABLE public.user_site_accesses ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.user_site_accesses ADD COLUMN IF NOT EXISTS user_id bigint;

ALTER TABLE public.user_site_accesses ADD COLUMN IF NOT EXISTS site_id bigint;

ALTER TABLE public.user_site_accesses ADD COLUMN IF NOT EXISTS role character varying(16);

CREATE SEQUENCE IF NOT EXISTS public.user_site_accesses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_site_accesses_id_seq OWNED BY public.user_site_accesses.id;

CREATE TABLE IF NOT EXISTS public.users (
    id bigint NOT NULL,
    username character varying(64) NOT NULL,
    password_hash character varying(255) NOT NULL,
    role character varying(16) DEFAULT 'viewer'::character varying NOT NULL,
    active boolean,
    last_login timestamp with time zone,
    created_at timestamp with time zone
);

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS id bigint;

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS username character varying(64);

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS password_hash character varying(255);

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS role character varying(16);

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS active boolean;

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS last_login timestamp with time zone;

ALTER TABLE public.users ADD COLUMN IF NOT EXISTS created_at timestamp with time zone;

CREATE SEQUENCE IF NOT EXISTS public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.alert_rules'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.alert_rules ALTER COLUMN id SET DEFAULT nextval('public.alert_rules_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.alerts'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.alerts ALTER COLUMN id SET DEFAULT nextval('public.alerts_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.annotations'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.annotations ALTER COLUMN id SET DEFAULT nextval('public.annotations_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.audit_logs'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.audit_logs ALTER COLUMN id SET DEFAULT nextval('public.audit_logs_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.dashboard_panels'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.dashboard_panels ALTER COLUMN id SET DEFAULT nextval('public.dashboard_panels_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.dashboards'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.dashboards ALTER COLUMN id SET DEFAULT nextval('public.dashboards_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.domains'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.domains ALTER COLUMN id SET DEFAULT nextval('public.domains_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.enrollment_tokens'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.enrollment_tokens ALTER COLUMN id SET DEFAULT nextval('public.enrollment_tokens_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.floor_plan_pins'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.floor_plan_pins ALTER COLUMN id SET DEFAULT nextval('public.floor_plan_pins_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.floor_plans'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.floor_plans ALTER COLUMN id SET DEFAULT nextval('public.floor_plans_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.log_entries'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.log_entries ALTER COLUMN id SET DEFAULT nextval('public.log_entries_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.metric_containers'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.metric_containers ALTER COLUMN id SET DEFAULT nextval('public.metric_containers_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.metric_load_balancers'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.metric_load_balancers ALTER COLUMN id SET DEFAULT nextval('public.metric_load_balancers_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.metric_server_trends'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.metric_server_trends ALTER COLUMN id SET DEFAULT nextval('public.metric_server_trends_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.metric_servers'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.metric_servers ALTER COLUMN id SET DEFAULT nextval('public.metric_servers_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.network_hosts'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.network_hosts ALTER COLUMN id SET DEFAULT nextval('public.network_hosts_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.sites'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.sites ALTER COLUMN id SET DEFAULT nextval('public.sites_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.user_site_accesses'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.user_site_accesses ALTER COLUMN id SET DEFAULT nextval('public.user_site_accesses_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_attrdef d
          JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
         WHERE d.adrelid = 'public.users'::regclass AND a.attname = 'id'
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'alert_rules_pkey' AND conrelid = 'public.alert_rules'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.alert_rules     ADD CONSTRAINT alert_rules_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'alert_states_pkey' AND conrelid = 'public.alert_states'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.alert_states     ADD CONSTRAINT alert_states_pkey PRIMARY KEY (key)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'alerts_pkey' AND conrelid = 'public.alerts'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.alerts     ADD CONSTRAINT alerts_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'annotations_pkey' AND conrelid = 'public.annotations'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.annotations     ADD CONSTRAINT annotations_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'audit_logs_pkey' AND conrelid = 'public.audit_logs'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.audit_logs     ADD CONSTRAINT audit_logs_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'containers_pkey' AND conrelid = 'public.containers'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.containers     ADD CONSTRAINT containers_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'dashboard_panels_pkey' AND conrelid = 'public.dashboard_panels'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.dashboard_panels     ADD CONSTRAINT dashboard_panels_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'dashboards_pkey' AND conrelid = 'public.dashboards'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.dashboards     ADD CONSTRAINT dashboards_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'device_credentials_pkey' AND conrelid = 'public.device_credentials'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.device_credentials     ADD CONSTRAINT device_credentials_pkey PRIMARY KEY (device_id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'domains_pkey' AND conrelid = 'public.domains'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.domains     ADD CONSTRAINT domains_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'enrollment_tokens_pkey' AND conrelid = 'public.enrollment_tokens'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.enrollment_tokens     ADD CONSTRAINT enrollment_tokens_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'floor_plan_pins_pkey' AND conrelid = 'public.floor_plan_pins'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.floor_plan_pins     ADD CONSTRAINT floor_plan_pins_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'floor_plans_pkey' AND conrelid = 'public.floor_plans'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.floor_plans     ADD CONSTRAINT floor_plans_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'log_entries_pkey' AND conrelid = 'public.log_entries'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.log_entries     ADD CONSTRAINT log_entries_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'metric_containers_pkey' AND conrelid = 'public.metric_containers'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.metric_containers     ADD CONSTRAINT metric_containers_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'metric_load_balancers_pkey' AND conrelid = 'public.metric_load_balancers'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.metric_load_balancers     ADD CONSTRAINT metric_load_balancers_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'metric_server_trends_pkey' AND conrelid = 'public.metric_server_trends'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.metric_server_trends     ADD CONSTRAINT metric_server_trends_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'metric_servers_pkey' AND conrelid = 'public.metric_servers'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.metric_servers     ADD CONSTRAINT metric_servers_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'network_hosts_pkey' AND conrelid = 'public.network_hosts'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.network_hosts     ADD CONSTRAINT network_hosts_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'servers_pkey' AND conrelid = 'public.servers'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.servers     ADD CONSTRAINT servers_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'sites_pkey' AND conrelid = 'public.sites'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.sites     ADD CONSTRAINT sites_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'uni_domains_name' AND conrelid = 'public.domains'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.domains     ADD CONSTRAINT uni_domains_name UNIQUE (name)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'user_sessions_pkey' AND conrelid = 'public.user_sessions'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.user_sessions     ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (token_hash)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'user_site_accesses_pkey' AND conrelid = 'public.user_site_accesses'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.user_site_accesses     ADD CONSTRAINT user_site_accesses_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'users_pkey' AND conrelid = 'public.users'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.users     ADD CONSTRAINT users_pkey PRIMARY KEY (id)$ddl$;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_alert_rules_depends_on_server_id ON public.alert_rules USING btree (depends_on_server_id);

CREATE INDEX IF NOT EXISTS idx_alert_rules_target_site_id ON public.alert_rules USING btree (target_site_id);

CREATE INDEX IF NOT EXISTS idx_alert_states_active ON public.alert_states USING btree (active);

CREATE INDEX IF NOT EXISTS idx_alert_states_last_notified_at ON public.alert_states USING btree (last_notified_at);

CREATE INDEX IF NOT EXISTS idx_alert_states_rule_id ON public.alert_states USING btree (rule_id);

CREATE INDEX IF NOT EXISTS idx_alert_states_server_id ON public.alert_states USING btree (server_id);

CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON public.alerts USING btree (created_at);

CREATE INDEX IF NOT EXISTS idx_alerts_delivery ON public.alerts USING btree (delivery);

CREATE INDEX IF NOT EXISTS idx_alerts_key ON public.alerts USING btree (key);

CREATE INDEX IF NOT EXISTS idx_alerts_next_attempt_at ON public.alerts USING btree (next_attempt_at);

CREATE INDEX IF NOT EXISTS idx_alerts_rule_id ON public.alerts USING btree (rule_id);

CREATE INDEX IF NOT EXISTS idx_alerts_server_id ON public.alerts USING btree (server_id);

CREATE INDEX IF NOT EXISTS idx_alerts_site_id ON public.alerts USING btree (site_id);

CREATE INDEX IF NOT EXISTS idx_alerts_status ON public.alerts USING btree (status);

CREATE INDEX IF NOT EXISTS idx_annotation_srv_at ON public.annotations USING btree (server_id, at);

CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON public.audit_logs USING btree (action);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_user_id ON public.audit_logs USING btree (actor_user_id);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_username ON public.audit_logs USING btree (actor_username);

CREATE INDEX IF NOT EXISTS idx_audit_logs_at ON public.audit_logs USING btree (at);

CREATE INDEX IF NOT EXISTS idx_audit_logs_result ON public.audit_logs USING btree (result);

CREATE INDEX IF NOT EXISTS idx_audit_logs_site_id ON public.audit_logs USING btree (site_id);

CREATE INDEX IF NOT EXISTS idx_audit_logs_source_ip ON public.audit_logs USING btree (source_ip);

CREATE INDEX IF NOT EXISTS idx_audit_logs_target_id ON public.audit_logs USING btree (target_id);

CREATE INDEX IF NOT EXISTS idx_audit_logs_target_type ON public.audit_logs USING btree (target_type);

CREATE INDEX IF NOT EXISTS idx_containers_docker_id ON public.containers USING btree (docker_id);

CREATE INDEX IF NOT EXISTS idx_containers_server_id ON public.containers USING btree (server_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_owner_name ON public.dashboards USING btree (owner_user_id, name);

CREATE INDEX IF NOT EXISTS idx_dashboard_panels_dashboard_id ON public.dashboard_panels USING btree (dashboard_id);

CREATE INDEX IF NOT EXISTS idx_device_credentials_machine_id ON public.device_credentials USING btree (machine_id);

CREATE INDEX IF NOT EXISTS idx_device_credentials_revoked_at ON public.device_credentials USING btree (revoked_at);

CREATE INDEX IF NOT EXISTS idx_device_credentials_site_id ON public.device_credentials USING btree (site_id);

CREATE INDEX IF NOT EXISTS idx_domains_invalid_reason ON public.domains USING btree (invalid_reason);

CREATE INDEX IF NOT EXISTS idx_domains_server_id ON public.domains USING btree (server_id);

CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_expires_at ON public.enrollment_tokens USING btree (expires_at);

CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_site_id ON public.enrollment_tokens USING btree (site_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_enrollment_tokens_token_hash ON public.enrollment_tokens USING btree (token_hash);

CREATE INDEX IF NOT EXISTS idx_floor_plan_pins_host_ip ON public.floor_plan_pins USING btree (host_ip);

CREATE INDEX IF NOT EXISTS idx_floor_plan_pins_plan_id ON public.floor_plan_pins USING btree (plan_id);

CREATE INDEX IF NOT EXISTS idx_floor_plans_site_id ON public.floor_plans USING btree (site_id);

CREATE INDEX IF NOT EXISTS idx_log_entries_container ON public.log_entries USING btree (container);

CREATE INDEX IF NOT EXISTS idx_log_entries_source ON public.log_entries USING btree (source);

CREATE INDEX IF NOT EXISTS idx_logentry_srv_ts ON public.log_entries USING btree (server_id, "timestamp" DESC);

CREATE INDEX IF NOT EXISTS idx_metric_load_balancers_server_id ON public.metric_load_balancers USING btree (server_id);

CREATE INDEX IF NOT EXISTS idx_metric_load_balancers_site_id ON public.metric_load_balancers USING btree (site_id);

CREATE INDEX IF NOT EXISTS idx_metric_load_balancers_timestamp ON public.metric_load_balancers USING btree ("timestamp");

CREATE INDEX IF NOT EXISTS idx_metriccontainer_ct_ts ON public.metric_containers USING btree (container_id, "timestamp" DESC);

CREATE INDEX IF NOT EXISTS idx_metricserver_srv_ts ON public.metric_servers USING btree (server_id, "timestamp" DESC);

CREATE INDEX IF NOT EXISTS idx_network_hosts_asset_tag ON public.network_hosts USING btree (asset_tag);

CREATE INDEX IF NOT EXISTS idx_network_hosts_device_type ON public.network_hosts USING btree (device_type);

CREATE INDEX IF NOT EXISTS idx_network_hosts_ip ON public.network_hosts USING btree (ip);

CREATE INDEX IF NOT EXISTS idx_network_hosts_last_seen ON public.network_hosts USING btree (last_seen);

CREATE INDEX IF NOT EXISTS idx_network_hosts_mac ON public.network_hosts USING btree (mac);

CREATE INDEX IF NOT EXISTS idx_network_hosts_sector ON public.network_hosts USING btree (sector);

CREATE INDEX IF NOT EXISTS idx_network_hosts_site_id ON public.network_hosts USING btree (site_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_network_hosts_site_ip ON public.network_hosts USING btree (COALESCE(site_id, (0)::bigint), ip);

CREATE INDEX IF NOT EXISTS idx_server_site_name ON public.servers USING btree (site_id, name);

CREATE INDEX IF NOT EXISTS idx_servers_deleted_at ON public.servers USING btree (deleted_at);

CREATE INDEX IF NOT EXISTS idx_servers_machine_id ON public.servers USING btree (machine_id);

CREATE INDEX IF NOT EXISTS idx_servers_site_id ON public.servers USING btree (site_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_servers_site_machine ON public.servers USING btree (COALESCE(site_id, (0)::bigint), machine_id) WHERE (((machine_id)::text <> ''::text) AND (deleted_at IS NULL));

CREATE UNIQUE INDEX IF NOT EXISTS idx_sites_code ON public.sites USING btree (code);

CREATE UNIQUE INDEX IF NOT EXISTS idx_trend_srv_bucket ON public.metric_server_trends USING btree (server_id, bucket);

CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON public.user_sessions USING btree (expires_at);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON public.user_sessions USING btree (user_id);

CREATE INDEX IF NOT EXISTS idx_user_sessions_username ON public.user_sessions USING btree (username);

CREATE INDEX IF NOT EXISTS idx_user_site_accesses_site_id ON public.user_site_accesses USING btree (site_id);

CREATE INDEX IF NOT EXISTS idx_user_site_accesses_user_id ON public.user_site_accesses USING btree (user_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON public.users USING btree (username);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conname = 'fk_dashboards_panels' AND conrelid = 'public.dashboard_panels'::regclass
    ) THEN
        EXECUTE $ddl$ALTER TABLE ONLY public.dashboard_panels     ADD CONSTRAINT fk_dashboards_panels FOREIGN KEY (dashboard_id) REFERENCES public.dashboards(id) ON DELETE CASCADE$ddl$;
    END IF;
END $$;
