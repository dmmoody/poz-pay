# Decision order

Pipelines for the hot path. Order is contractual for the spike.

## Check rail

```text
1. Resolve account by (fi_id, account_id)
   - missing / wrong FI → ACCOUNT_NOT_OURS → return
   - inactive → ACCOUNT_INACTIVE → return
2. Lookup register item (fi_id, account_id, check_number) where not voided
   - missing → CHECK_NOT_IN_REGISTER → return
   - voided → CHECK_VOIDED → return
   - posted (`paid_at` set) → CHECK_ALREADY_PAID → return
   - register issue_date older than FI/party `max_age_days` (default 180) → CHECK_EXPIRED → return
     (skipped when register has no issue_date)
3. Accumulate field mismatches (list all that fail)
   - amount → AMOUNT_MISMATCH
   - payee → PAYEE_MISMATCH
   - issue date (if present + tolerance configured) → ISSUE_DATE_MISMATCH
4. Decide
   - any mismatch → `review`, or `return` when party/FI `return_on_mismatch` is true
   - none → `pay` with `match` `REGISTER_MATCH` (`register_item_id`)
```

Each call appends one `decision_audit` row (`check_number`, `register_item_id` when the register row resolved, request, decision). Calling decide again before the item is posted returns the same `pay` and adds another row.

Gates stop before field matching. Field mismatches always list every failing check.

## Posting

`paid_at` is settlement, written only by `POST /v1/fis/{fi_id}/accounts/{account_id}/check-postings`. A repeated post returns the original `paid_at` and `paid_correlation_id`. After that, decide returns `CHECK_ALREADY_PAID`.

`GET /v1/fis/{fi_id}/accounts/{account_id}/checks/{check_number}/decisions` returns the audit rows for that check, oldest first, plus `paid_at` when posted.

## ACH rail

```text
1. Resolve account (same as check)
2. Load party + FI ACH settings (approve mode)
3. Active whitelist match for party (and account scope if set)?
   - full match → pay; skip rules unless FI flag force_rules_after_whitelist=true (default false)
   - partial match (originator hits, amount/window/SEC fail) → accumulate exceptions; do not auto-pay
4. Evaluate party rule_set (ordered by priority ascending)
   - first rule whose predicate matches and action ≠ continue → apply action and stop rules
   - action pay → pay
   - action return → return (+ RULE_RETURN / ORIGINATOR_DENIED as coded)
   - action review → review
   - action continue → keep going
5. If no party terminating rule: evaluate FI rule_set the same way
6. If still undecided: apply approve mode
   - approve_all → pay
   - approve_unknown → pay if only “unknown” style findings; else review
   - manual_review → review (+ APPROVE_MODE_REVIEW)
   - strict → return (+ APPROVE_MODE_STRICT)
```

### Tier precedence

- **Whitelist** is checked before rules (fast allow).
- **Party rules beat FI rules** for terminating actions: if a party rule matches with `pay|return|review`, FI rules are not consulted.
- FI rules fill gaps when party has no match / only `continue`.
- **Approve mode** is the last resort after whitelist + rules.

### Conflict note

If whitelist partially matches and rules later `pay`, spike prefers **rules terminating action** only when whitelist did not fully match. Full whitelist match short-circuits to `pay`.
