ALTER TABLE alerts DROP CONSTRAINT IF EXISTS ck_alerts_delivery;

ALTER TABLE alerts
    ADD CONSTRAINT ck_alerts_delivery CHECK (delivery IN ('pendente', 'enviado', 'falhou', 'sem_canal'));
