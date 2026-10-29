-- FI/party max age from register issue_date (UCC-aligned default: 180 days ≈ 6 months).

ALTER TABLE fi_check_settings
    ADD COLUMN max_age_days integer NOT NULL DEFAULT 180
        CONSTRAINT fi_check_settings_max_age_days_check CHECK (max_age_days > 0);

ALTER TABLE party_check_settings
    ADD COLUMN max_age_days integer NOT NULL DEFAULT 180
        CONSTRAINT party_check_settings_max_age_days_check CHECK (max_age_days > 0);

COMMENT ON COLUMN fi_check_settings.max_age_days IS
    'Return CHECK_EXPIRED when register issue_date is older than this many days (default 180 ≈ UCC 6 months)';
COMMENT ON COLUMN party_check_settings.max_age_days IS
    'Party override for max_age_days; when row exists, wins over FI';

INSERT INTO exception_codes (code, label, rail) VALUES
    ('CHECK_EXPIRED', 'Check past allowed age from issue date', 'check');
