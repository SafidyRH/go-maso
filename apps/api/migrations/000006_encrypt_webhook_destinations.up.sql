ALTER TABLE alert_rules
    DROP CONSTRAINT alert_rules_check,
    ADD COLUMN webhook_url_encrypted BYTEA NOT NULL DEFAULT ''::bytea,
    ADD CONSTRAINT alert_rules_enabled_webhook_check
        CHECK (NOT enabled OR webhook_url <> '' OR OCTET_LENGTH(webhook_url_encrypted) > 0);

ALTER TABLE notification_deliveries
    ADD COLUMN webhook_url_encrypted BYTEA NOT NULL DEFAULT ''::bytea;
