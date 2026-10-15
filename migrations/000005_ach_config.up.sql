-- ACH settings, rule sets/rules, whitelist. Party settings override FI when present.

CREATE TABLE fi_ach_settings (
    fi_id           uuid PRIMARY KEY REFERENCES financial_institutions (id) ON DELETE CASCADE,
    approve_mode    text NOT NULL REFERENCES approve_modes (code),
    default_tz      text NOT NULL DEFAULT 'America/New_York',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE party_ach_settings (
    party_id        uuid PRIMARY KEY,
    fi_id           uuid NOT NULL,
    approve_mode    text NOT NULL REFERENCES approve_modes (code),
    default_tz      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT party_ach_settings_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE CASCADE
);

CREATE TABLE rule_sets (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fi_id       uuid NOT NULL REFERENCES financial_institutions (id) ON DELETE CASCADE,
    party_id    uuid,
    tier        text NOT NULL REFERENCES rule_set_tiers (code),
    name        text NOT NULL,
    enabled     boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT rule_sets_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT rule_sets_tier_shape CHECK (
        (tier = 'fi' AND party_id IS NULL)
        OR (tier = 'party' AND party_id IS NOT NULL)
    ),
    CONSTRAINT rule_sets_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE CASCADE
);

CREATE INDEX rule_sets_fi_tier_idx ON rule_sets (fi_id, tier) WHERE enabled;
CREATE INDEX rule_sets_party_idx ON rule_sets (fi_id, party_id) WHERE enabled AND party_id IS NOT NULL;

-- One enabled default set per FI tier / party (spike simplicity).
CREATE UNIQUE INDEX rule_sets_fi_default_uq
    ON rule_sets (fi_id)
    WHERE tier = 'fi' AND enabled;

CREATE UNIQUE INDEX rule_sets_party_default_uq
    ON rule_sets (fi_id, party_id)
    WHERE tier = 'party' AND enabled AND party_id IS NOT NULL;

CREATE TABLE rules (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_set_id uuid NOT NULL REFERENCES rule_sets (id) ON DELETE CASCADE,
    priority    integer NOT NULL,
    name        text NOT NULL,
    predicate   jsonb NOT NULL DEFAULT '{}'::jsonb,
    action      text NOT NULL REFERENCES rule_actions (code),
    enabled     boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT rules_name_nonempty CHECK (length(trim(name)) > 0),
    CONSTRAINT rules_priority_nonneg CHECK (priority >= 0),
    CONSTRAINT rules_predicate_object CHECK (jsonb_typeof(predicate) = 'object')
);

CREATE UNIQUE INDEX rules_set_priority_uq ON rules (rule_set_id, priority);
CREATE INDEX rules_set_enabled_idx ON rules (rule_set_id, priority) WHERE enabled;

CREATE TABLE ach_whitelist_entries (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    fi_id            uuid NOT NULL REFERENCES financial_institutions (id) ON DELETE CASCADE,
    party_id         uuid NOT NULL,
    account_id       uuid,
    originator_id    text NOT NULL,
    originator_name  text,
    amount_min       bigint,
    amount_max       bigint,
    sec_codes        text[] NOT NULL DEFAULT '{}',
    debit_credit     text[] NOT NULL DEFAULT '{}',
    time_windows     jsonb NOT NULL DEFAULT '[]'::jsonb,
    status           text NOT NULL REFERENCES whitelist_statuses (code),
    effective_from   timestamptz NOT NULL DEFAULT now(),
    effective_to     timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ach_whitelist_originator_nonempty CHECK (length(trim(originator_id)) > 0),
    CONSTRAINT ach_whitelist_amount_bounds CHECK (
        amount_min IS NULL OR amount_min >= 0
    ),
    CONSTRAINT ach_whitelist_amount_range CHECK (
        amount_min IS NULL OR amount_max IS NULL OR amount_min <= amount_max
    ),
    CONSTRAINT ach_whitelist_effective_range CHECK (
        effective_to IS NULL OR effective_to > effective_from
    ),
    CONSTRAINT ach_whitelist_time_windows_array CHECK (jsonb_typeof(time_windows) = 'array'),
    CONSTRAINT ach_whitelist_party_fi_fk
        FOREIGN KEY (party_id, fi_id) REFERENCES parties (id, fi_id) ON DELETE CASCADE,
    CONSTRAINT ach_whitelist_account_fi_fk
        FOREIGN KEY (account_id, fi_id) REFERENCES accounts (id, fi_id) ON DELETE CASCADE
);

CREATE INDEX ach_whitelist_hot_idx
    ON ach_whitelist_entries (fi_id, party_id, originator_id)
    WHERE status = 'active';

CREATE INDEX ach_whitelist_status_idx
    ON ach_whitelist_entries (fi_id, party_id, status);
