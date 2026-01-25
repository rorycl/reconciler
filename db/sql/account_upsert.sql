/*
 Reconciler app SQL
 account_upsert.sql
 Upsert a Xero Account into the database.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
         'acc-9998'                   AS AccountID     /* @param */
         ,'9998'                      AS Code          /* @param */
         ,'Arbitrary Too'             AS Name          /* @param */
         ,'Another Arbitrary Account' AS Description   /* @param */
         ,'LIABILITY'                 AS Type          /* @param */
         ,''                          AS TaxType       /* @param */
         ,'ACTIvE'                    AS Status        /* @param */
         ,'false'                     AS SystemAccount /* @param */
         ,'GBP'                       AS CurrencyCode  /* @param */
         ,'2026-01-02'                AS Updated       /* @param */
)

INSERT INTO accounts (
	id
    ,code
    ,name
    ,description
    ,type
    ,tax_type
    ,status
    ,system_account
    ,currency_code
    ,updated_at
)
SELECT
    v.AccountID    
    ,v.Code         
    ,v.Name         
    ,v.Description  
    ,v.Type         
    ,v.TaxType      
    ,v.Status       
    ,v.SystemAccount
    ,v.CurrencyCode 
    ,v.Updated      
FROM
    variables v
-- sqlite.org/lang_upsert.html PARSING AMBIGUITY
WHERE
    true
ON CONFLICT (id) DO UPDATE SET
    code           = excluded.code
   ,name           = excluded.name
   ,description    = excluded.description
   ,type           = excluded.type
   ,tax_type       = excluded.tax_type
   ,status         = excluded.status
   ,system_account = excluded.system_account
   ,currency_code  = excluded.currency_code
   ,updated_at     = excluded.updated_at
;
