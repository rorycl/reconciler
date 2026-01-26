/*
 Reconciler app SQL
 bank_transaction_upsert.sql
 Upsert a Xero Bank Transaction into the database.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/
WITH variables AS (
    SELECT
         'bt-001'                      AS BankTransactionID    /* @param */
         ,'RECEIVE'                    AS Type                 /* @param */
         ,'RECONCILED'                 AS Status               /* @param */
         ,'JG-PAYOUT-2025-04-15b'      AS Reference            /* @param */
         ,338.50                       AS Total                /* @param */
         ,false                        AS IsReconciled         /* @param */
         ,date('2025-04-15T14:00:01Z') AS Date                 /* @param */
         ,date('2026-01-01')           AS Updated              /* @param */
         ,'Admin User'                 AS Contact              /* @param */
         ,'Current Account'            AS BankAccount          /* @param */
)
INSERT INTO bank_transactions (
    id
    ,type
    ,status
    ,reference
    ,total
    ,is_reconciled
    ,date
    ,updated_at
    ,contact
    ,bank_account
)
SELECT
    v.BankTransactionID   
    ,v.Type                
    ,v.Status              
    ,v.Reference           
    ,v.Total               
    ,v.IsReconciled        
    ,v.Date                
    ,v.Updated             
    ,v.Contact
    ,v.BankAccount         
FROM
    variables v
-- sqlite.org/lang_upsert.html PARSING AMBIGUITY
WHERE
    true
ON CONFLICT (id) DO UPDATE SET
    type           = excluded.type
    ,status        = excluded.status
    ,reference     = excluded.reference
    ,total         = excluded.total
    ,is_reconciled = excluded.is_reconciled
    ,date          = excluded.date
    ,updated_at    = excluded.updated_at
    ,contact       = excluded.contact
    ,bank_account  = excluded.bank_account
;
