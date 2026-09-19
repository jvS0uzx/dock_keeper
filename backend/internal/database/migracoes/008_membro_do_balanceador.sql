ALTER TABLE servers ADD COLUMN IF NOT EXISTS behind_lb boolean;

CREATE INDEX IF NOT EXISTS idx_metric_lb_ts_upstream
    ON metric_load_balancers (timestamp DESC, upstream_addr);
