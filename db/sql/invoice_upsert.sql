INSERT INTO invoices (
	id
    ,type
    ,status
    ,invoice_number
    ,reference
    ,total
    ,amount_paid
    ,date
    ,updated_at
    ,contact_id
    ,contact_name
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
)
ON CONFLICT (id) DO UPDATE SET
    type            = excluded.type
    ,status         = excluded.status
    ,invoice_number = excluded.invoice_number
    ,reference      = excluded.reference
    ,total          = excluded.total
    ,amount_paid    = excluded.amount_paid
    ,date           = excluded.date
    ,updated_at     = excluded.updated_at
    ,contact_id     = excluded.contact_id
    ,contact_name   = excluded.contact_name
;
