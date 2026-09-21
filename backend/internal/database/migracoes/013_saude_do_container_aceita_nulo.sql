ALTER TABLE metric_containers ALTER COLUMN health DROP DEFAULT;
ALTER TABLE metric_containers ALTER COLUMN health DROP NOT NULL;
ALTER TABLE metric_containers ALTER COLUMN restart_count DROP DEFAULT;
ALTER TABLE metric_containers ALTER COLUMN restart_count DROP NOT NULL;
ALTER TABLE metric_containers ALTER COLUMN oom_killed DROP DEFAULT;
ALTER TABLE metric_containers ALTER COLUMN oom_killed DROP NOT NULL;

UPDATE metric_containers SET health = NULL, restart_count = NULL, oom_killed = NULL;
