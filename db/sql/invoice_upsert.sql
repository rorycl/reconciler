/*
 Reconciler app SQL
 invoice_upsert.sql
 Upsert a Xero Invoice into the database.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
         'inv-001'           AS InvoiceID     /* @param */
         ,'ACCREC'           AS Type          /* @param */
         ,'AUTHORISED'       AS Status        /* @param */
         ,'INV-2025-101b'    AS InvoiceNumber /* @param */
         ,'Example Ref'      AS Reference     /* @param */
         ,499.99             AS Total         /* @param */
         ,498.98             AS AmountPaid    /* @param */
         ,date('2025-09-01') AS Date          /* @param */
         ,date('2026-01-01') AS Updated       /* @param */
         ,'Test User'        AS Contact /* @param */
)
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
    ,contact
)
SELECT
    v.InvoiceID    
    ,v.Type         
    ,v.Status       
    ,v.InvoiceNumber
    ,v.Reference    
    ,v.Total        
    ,v.AmountPaid   
    ,v.Date         
    ,v.Updated      
    ,v.Contact
FROM
    variables v
-- sqlite.org/lang_upsert.html PARSING AMBIGUITY
WHERE
    true
ON CONFLICT (id) DO UPDATE SET
    type            = excluded.type
    ,status         = excluded.status
    ,invoice_number = excluded.invoice_number
    ,reference      = excluded.reference
    ,total          = excluded.total
    ,amount_paid    = excluded.amount_paid
    ,date           = excluded.date
    ,updated_at     = excluded.updated_at
    ,contact        = excluded.contact
;
