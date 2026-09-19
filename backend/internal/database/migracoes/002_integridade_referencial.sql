CREATE TABLE IF NOT EXISTS migracao_limpeza (
    id bigserial PRIMARY KEY,
    versao integer NOT NULL,
    tabela text NOT NULL,
    motivo text NOT NULL,
    linhas bigint NOT NULL,
    executada_em timestamp with time zone NOT NULL DEFAULT now()
);

DO $$
DECLARE n bigint;
BEGIN
    DELETE FROM device_credentials c
     WHERE NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = c.site_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'device_credentials', 'credencial de unidade inexistente', n);

    DELETE FROM enrollment_tokens t
     WHERE NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = t.site_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'enrollment_tokens', 'convite de unidade inexistente', n);

    DELETE FROM user_site_accesses a
     WHERE a.site_id IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = a.site_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'user_site_accesses', 'concessão para unidade inexistente', n);

    DELETE FROM user_site_accesses a
     WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = a.user_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'user_site_accesses', 'concessão de usuário inexistente', n);

    DELETE FROM alert_states st
     WHERE NOT EXISTS (SELECT 1 FROM alert_rules r WHERE r.id = st.rule_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'alert_states', 'estado de regra inexistente', n);

    DELETE FROM floor_plan_pins p
     WHERE NOT EXISTS (SELECT 1 FROM floor_plans f WHERE f.id = p.plan_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'floor_plan_pins', 'marcador de planta inexistente', n);

    DELETE FROM dashboards d
     WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = d.owner_user_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'dashboards', 'painel de usuário inexistente', n);

    UPDATE servers sv SET site_id = NULL
     WHERE sv.site_id IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = sv.site_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'servers', 'unidade inexistente virou nulo', n);

    UPDATE alert_rules r SET target_site_id = NULL
     WHERE r.target_site_id IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM sites s WHERE s.id = r.target_site_id);
    GET DIAGNOSTICS n = ROW_COUNT;
    INSERT INTO migracao_limpeza (versao, tabela, motivo, linhas)
         VALUES (2, 'alert_rules', 'unidade alvo inexistente virou nulo', n);
END $$;

ALTER TABLE device_credentials
    ADD CONSTRAINT fk_device_credentials_site
    FOREIGN KEY (site_id) REFERENCES sites (id) ON DELETE RESTRICT;

ALTER TABLE enrollment_tokens
    ADD CONSTRAINT fk_enrollment_tokens_site
    FOREIGN KEY (site_id) REFERENCES sites (id) ON DELETE RESTRICT;

ALTER TABLE user_site_accesses
    ADD CONSTRAINT fk_user_site_accesses_site
    FOREIGN KEY (site_id) REFERENCES sites (id) ON DELETE CASCADE;

ALTER TABLE user_site_accesses
    ADD CONSTRAINT fk_user_site_accesses_user
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE;

ALTER TABLE alert_states
    ADD CONSTRAINT fk_alert_states_rule
    FOREIGN KEY (rule_id) REFERENCES alert_rules (id) ON DELETE CASCADE;

ALTER TABLE floor_plan_pins
    ADD CONSTRAINT fk_floor_plan_pins_plan
    FOREIGN KEY (plan_id) REFERENCES floor_plans (id) ON DELETE CASCADE;

ALTER TABLE dashboards
    ADD CONSTRAINT fk_dashboards_owner
    FOREIGN KEY (owner_user_id) REFERENCES users (id) ON DELETE CASCADE;

ALTER TABLE servers
    ADD CONSTRAINT fk_servers_site
    FOREIGN KEY (site_id) REFERENCES sites (id) ON DELETE SET NULL;

ALTER TABLE alert_rules
    ADD CONSTRAINT fk_alert_rules_target_site
    FOREIGN KEY (target_site_id) REFERENCES sites (id) ON DELETE SET NULL;
