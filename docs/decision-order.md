# Decision order

Pipelines for the hot path. Order is contractual for the spike.

## Check rail

```text
1. Resolve account by (fi_id, account_id)
   - missing / wrong FI → hard ACCOUNT_NOT_OURS → return
   - inactive → hard ACCOUNT_INACTIVE → return
2. Lookup register item (fi_id, account_id, check_number) where not voided
   - missing → hard CHECK_NOT_IN_REGISTER → return
   - voided → hard CHECK_VOIDED → return
   - already paid (if tracked) → hard CHECK_ALREADY_PAID → return
3. Accumulate soft matches
   - amount → AMOUNT_MISMATCH
   - payee → PAYEE_MISMATCH
   - issue date (if present + tolerance configured) → ISSUE_DATE_MISMATCH
4. Decide
   - any soft → `review`, or `return` when party/FI `return_on_mismatch` is true
   - none → `pay`, then set register `paid_at` (replay → `CHECK_ALREADY_PAID`)
```

Hard stops do **not** continue soft matching.

## ACH rail

```text
1. Resolve account (same as check)
2. Load party + FI ACH settings (approve mode)
3. Active whitelist match for party (and account scope if set)?
   - full match → pay (record reason WHITELIST_MATCH); skip rules unless FI flag force_rules_after_whitelist=true (default false)
   - partial match (originator hits, amount/window/SEC fail) → accumulate soft exceptions; do not auto-pay
4. Evaluate party rule_set (ordered by priority ascending)
   - first rule whose predicate matches and action ≠ continue → apply action and stop rules
   - action pay → pay
   - action return → return (+ RULE_RETURN / ORIGINATOR_DENIED as coded)
   - action review → review
   - action continue → keep going
5. If no party terminating rule: evaluate FI rule_set the same way
6. If still undecided: apply approve mode
   - approve_all → pay
   - approve_unknown → pay if only “unknown” style softs; else review
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
