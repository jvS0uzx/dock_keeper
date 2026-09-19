DO $$
DECLARE n bigint;
BEGIN
    SELECT count(*) INTO n FROM users WHERE role NOT IN ('viewer', 'operator', 'admin');
    IF n > 0 THEN
        RAISE EXCEPTION 'users.role tem % linha(s) fora de viewer, operator e admin; corrija antes de migrar', n;
    END IF;

    SELECT count(*) INTO n FROM user_site_accesses WHERE role NOT IN ('viewer', 'operator', 'admin');
    IF n > 0 THEN
        RAISE EXCEPTION 'user_site_accesses.role tem % linha(s) fora de viewer, operator e admin', n;
    END IF;

    SELECT count(*) INTO n FROM device_credentials WHERE kind NOT IN ('agent', 'collector');
    IF n > 0 THEN
        RAISE EXCEPTION 'device_credentials.kind tem % linha(s) fora de agent e collector', n;
    END IF;

    SELECT count(*) INTO n FROM enrollment_tokens WHERE kind NOT IN ('agent', 'collector');
    IF n > 0 THEN
        RAISE EXCEPTION 'enrollment_tokens.kind tem % linha(s) fora de agent e collector', n;
    END IF;

    SELECT count(*) INTO n FROM alert_rules
     WHERE metric NOT IN ('cpu', 'mem', 'disk', 'load', 'temperature', 'net_rx', 'net_tx', 'rtt');
    IF n > 0 THEN
        RAISE EXCEPTION 'alert_rules.metric tem % linha(s) com métrica que o motor não avalia', n;
    END IF;

    SELECT count(*) INTO n FROM alert_rules WHERE operator NOT IN ('>', '<');
    IF n > 0 THEN
        RAISE EXCEPTION 'alert_rules.operator tem % linha(s) fora de > e <', n;
    END IF;

    SELECT count(*) INTO n FROM alert_rules
     WHERE severity NOT IN ('info', 'warning', 'high', 'critical');
    IF n > 0 THEN
        RAISE EXCEPTION 'alert_rules.severity tem % linha(s) fora de info, warning, high e critical', n;
    END IF;

    SELECT count(*) INTO n FROM alerts
     WHERE severity NOT IN ('info', 'warning', 'high', 'critical');
    IF n > 0 THEN
        RAISE EXCEPTION 'alerts.severity tem % linha(s) fora de info, warning, high e critical', n;
    END IF;

    SELECT count(*) INTO n FROM alerts WHERE status NOT IN ('open', 'acked', 'resolved');
    IF n > 0 THEN
        RAISE EXCEPTION 'alerts.status tem % linha(s) fora de open, acked e resolved', n;
    END IF;

    SELECT count(*) INTO n FROM alerts WHERE delivery NOT IN ('pendente', 'enviado', 'falhou');
    IF n > 0 THEN
        RAISE EXCEPTION 'alerts.delivery tem % linha(s) fora de pendente, enviado e falhou', n;
    END IF;
END $$;

UPDATE alert_states SET severity = '' WHERE severity IS NULL;

DO $$
DECLARE n bigint;
BEGIN
    SELECT count(*) INTO n FROM alert_states
     WHERE severity NOT IN ('', 'info', 'warning', 'high', 'critical');
    IF n > 0 THEN
        RAISE EXCEPTION 'alert_states.severity tem % linha(s) com severidade desconhecida', n;
    END IF;
END $$;

ALTER TABLE users
    ADD CONSTRAINT ck_users_role CHECK (role IN ('viewer', 'operator', 'admin'));

ALTER TABLE user_site_accesses
    ADD CONSTRAINT ck_user_site_accesses_role CHECK (role IN ('viewer', 'operator', 'admin'));

ALTER TABLE device_credentials
    ADD CONSTRAINT ck_device_credentials_kind CHECK (kind IN ('agent', 'collector'));

ALTER TABLE enrollment_tokens
    ADD CONSTRAINT ck_enrollment_tokens_kind CHECK (kind IN ('agent', 'collector'));

ALTER TABLE alert_rules
    ADD CONSTRAINT ck_alert_rules_metric
    CHECK (metric IN ('cpu', 'mem', 'disk', 'load', 'temperature', 'net_rx', 'net_tx', 'rtt'));

ALTER TABLE alert_rules
    ADD CONSTRAINT ck_alert_rules_operator CHECK (operator IN ('>', '<'));

ALTER TABLE alert_rules
    ADD CONSTRAINT ck_alert_rules_severity
    CHECK (severity IN ('info', 'warning', 'high', 'critical'));

ALTER TABLE alert_states
    ADD CONSTRAINT ck_alert_states_severity
    CHECK (severity IN ('', 'info', 'warning', 'high', 'critical'));

ALTER TABLE alerts
    ADD CONSTRAINT ck_alerts_severity
    CHECK (severity IN ('info', 'warning', 'high', 'critical'));

ALTER TABLE alerts
    ADD CONSTRAINT ck_alerts_status CHECK (status IN ('open', 'acked', 'resolved'));

ALTER TABLE alerts
    ADD CONSTRAINT ck_alerts_delivery CHECK (delivery IN ('pendente', 'enviado', 'falhou'));
