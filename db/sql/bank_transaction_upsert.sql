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
         ,'7404f143aa1c'               AS ContactID            /* @param */
         ,'Admin User'                 AS ContactName          /* @param */
         ,'4c37c386'                   AS BankAccountAccountID /* @param */
         ,'Current Account'            AS BankAccountName      /* @param */
         ,'00333332'                   AS BankAccountCode      /* @param */
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
    ,contact_id
    ,contact_name
    ,bank_account_id
    ,bank_account_name
    ,bank_account_code
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
    ,v.ContactID           
    ,v.ContactName         
    ,v.BankAccountAccountID
    ,v.BankAccountName     
    ,v.BankAccountCode     
FROM
    variables v
-- https://sqlite.org/lang_upsert.html PARSING AMBIGUITY
WHERE
    true
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
