-- Reference / lookup tables. Codes are stable API tokens; labels are display-only.

CREATE TABLE party_types (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rail_types (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE decision_codes (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE exception_severities (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE exception_codes (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    severity    text NOT NULL REFERENCES exception_severities (code),
    rail        text REFERENCES rail_types (code), -- NULL = shared across rails
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE approve_modes (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rule_set_tiers (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rule_actions (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE whitelist_statuses (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE debit_credit_types (
    code        text PRIMARY KEY,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

INSERT INTO party_types (code, label) VALUES
    ('business', 'Business'),
    ('treasury', 'Treasury'),
    ('consumer', 'Consumer'),
    ('other', 'Other');

INSERT INTO rail_types (code, label) VALUES
    ('check', 'Check'),
    ('ach', 'ACH');

INSERT INTO decision_codes (code, label) VALUES
    ('pay', 'Pay'),
    ('return', 'Return'),
    ('review', 'Review');

INSERT INTO exception_severities (code, label) VALUES
    ('hard', 'Hard (first-fail)'),
    ('soft', 'Soft (accumulate)');

INSERT INTO exception_codes (code, label, severity, rail) VALUES
    ('ACCOUNT_NOT_OURS', 'Account not found or not owned by FI', 'hard', NULL),
    ('ACCOUNT_INACTIVE', 'Account inactive for positive pay', 'hard', NULL),
    ('CHECK_NOT_IN_REGISTER', 'Check not in register', 'hard', 'check'),
    ('CHECK_VOIDED', 'Check voided in register', 'hard', 'check'),
    ('CHECK_ALREADY_PAID', 'Check already paid/presented', 'hard', 'check'),
    ('AMOUNT_MISMATCH', 'Amount does not match register', 'soft', 'check'),
    ('PAYEE_MISMATCH', 'Payee does not match register', 'soft', 'check'),
    ('ISSUE_DATE_MISMATCH', 'Issue date outside tolerance', 'soft', 'check'),
    ('ORIGINATOR_DENIED', 'Originator denied by rule', 'hard', 'ach'),
    ('ORIGINATOR_UNKNOWN', 'Originator not known', 'soft', 'ach'),
    ('AMOUNT_OUT_OF_BOUNDS', 'Amount outside allowed bounds', 'soft', 'ach'),
    ('OUTSIDE_TIME_WINDOW', 'Outside allowed time window', 'soft', 'ach'),
    ('SEC_NOT_ALLOWED', 'SEC code not allowed', 'soft', 'ach'),
    ('DEBIT_CREDIT_MISMATCH', 'Debit/credit not allowed', 'soft', 'ach'),
    ('RULE_RETURN', 'Rule action return', 'hard', 'ach'),
    ('RULE_REVIEW', 'Rule action review', 'soft', 'ach'),
    ('APPROVE_MODE_REVIEW', 'Approve mode requires review', 'soft', 'ach'),
    ('APPROVE_MODE_STRICT', 'Approve mode strict return', 'hard', 'ach');

INSERT INTO approve_modes (code, label) VALUES
    ('approve_all', 'Approve all undecided'),
    ('approve_unknown', 'Approve unknown originators'),
    ('manual_review', 'Manual review'),
    ('strict', 'Strict return');

INSERT INTO rule_set_tiers (code, label) VALUES
    ('fi', 'Financial institution'),
    ('party', 'Party');

INSERT INTO rule_actions (code, label) VALUES
    ('pay', 'Pay'),
    ('return', 'Return'),
    ('review', 'Review'),
    ('continue', 'Continue');

INSERT INTO whitelist_statuses (code, label) VALUES
    ('active', 'Active'),
    ('proposed', 'Proposed (learning)'),
    ('rejected', 'Rejected');

INSERT INTO debit_credit_types (code, label) VALUES
    ('debit', 'Debit'),
    ('credit', 'Credit');
