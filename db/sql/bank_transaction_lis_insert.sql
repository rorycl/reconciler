/*
 Reconciler app SQL
 bank_transaction_lis_insert.sql
 Insert a bank transaction line item.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
         'bt-li-unrec-01d'       AS LineItemID        /* @param */
         ,'bt-001'               AS BankTransactionID /* @param */
         ,'JustGiving Anonymous' AS Description       /* @param */
         ,1                      AS Quantity          /* @param */
         ,1.20                   AS UnitAmount        /* @param */
         ,1.20                   AS LineAmount        /* @param */
         ,'9999'                 AS AccountCode       /* @param */
         ,0.20                   AS TaxAmount         /* @param */
)
INSERT INTO bank_transaction_line_items (
    id
    ,transaction_id
    ,description
    ,quantity
    ,unit_amount
    ,line_amount
    ,account_code
    ,tax_amount
)
SELECT
    v.LineItemID        
    ,v.BankTransactionID 
    ,v.Description       
    ,v.Quantity          
    ,v.UnitAmount        
    ,v.LineAmount        
    ,v.AccountCode       
    ,v.TaxAmount         
FROM
    variables v
;
