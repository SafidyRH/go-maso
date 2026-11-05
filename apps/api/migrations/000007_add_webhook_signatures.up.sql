ALTER TABLE alert_rules
    ADD COLUMN signing_secret_encrypted BYTEA NOT NULL DEFAULT ''::bytea;

ALTER TABLE notification_deliveries
    ADD COLUMN signing_secret_encrypted BYTEA NOT NULL DEFAULT ''::bytea;
