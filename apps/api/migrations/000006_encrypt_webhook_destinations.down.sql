UPDATE alert_rules
SET enabled = FALSE
WHERE webhook_url = '' AND OCTET_LENGTH(webhook_url_encrypted) > 0;

ALTER TABLE notification_deliveries
    DROP COLUMN webhook_url_encrypted;

ALTER TABLE alert_rules
    DROP CONSTRAINT alert_rules_enabled_webhook_check,
    DROP COLUMN webhook_url_encrypted,
    ADD CHECK (NOT enabled OR webhook_url <> '');
