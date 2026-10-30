DELETE FROM exception_codes WHERE code = 'CHECK_EXPIRED';

ALTER TABLE party_check_settings
    DROP CONSTRAINT IF EXISTS party_check_settings_max_age_days_check,
    DROP COLUMN IF EXISTS max_age_days;

ALTER TABLE fi_check_settings
    DROP CONSTRAINT IF EXISTS fi_check_settings_max_age_days_check,
    DROP COLUMN IF EXISTS max_age_days;
