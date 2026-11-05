ALTER TABLE notification_deliveries
    DROP COLUMN signing_secret_encrypted;

ALTER TABLE alert_rules
    DROP COLUMN signing_secret_encrypted;
