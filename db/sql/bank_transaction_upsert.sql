INSERT INTO bank_transactions (
    id
    ,type
    ,status
    ,reference
    ,total
    ,is_reconciled
    ,date
    ,updated_at
    ,contact_id
    ,contact_name
    ,bank_account_id
    ,bank_account_name
    ,bank_account_code
)
VALUES (
    ?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
)
ON CONFLICT (id) DO UPDATE SET
    type               = excluded.type
    ,status            = excluded.status
    ,reference         = excluded.reference
    ,total             = excluded.total
    ,is_reconciled     = excluded.is_reconciled
    ,date              = excluded.date
    ,updated_at        = excluded.updated_at
    ,contact_id        = excluded.contact_id
    ,contact_name      = excluded.contact_name
    ,bank_account_id   = excluded.bank_account_id
    ,bank_account_name = excluded.bank_account_name
    ,bank_account_code = excluded.bank_account_code
;
