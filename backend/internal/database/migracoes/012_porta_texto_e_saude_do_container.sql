ALTER TABLE network_hosts ALTER COLUMN open_ports TYPE text;

ALTER TABLE metric_containers ADD COLUMN IF NOT EXISTS health character varying(32) NOT NULL DEFAULT '';
ALTER TABLE metric_containers ADD COLUMN IF NOT EXISTS restart_count integer NOT NULL DEFAULT 0;
ALTER TABLE metric_containers ADD COLUMN IF NOT EXISTS oom_killed boolean NOT NULL DEFAULT false;
