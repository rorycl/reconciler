/*
 Reconciler app SQL
 invoice_lis_insert.sql
 Insert an invoice line item.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
     'inv-li-002c'           AS LineItemID     /* @param */
     ,'inv-002'              AS InvoiceID      /* @param */
     ,'Donation for Q1 2025' AS Description    /* @param */
     ,1                      AS Quantity       /* @param */
     ,200.0                  AS UnitAmount     /* @param */
     ,200.0                  AS LineAmount     /* @param */
     ,5501                   AS AccountCode    /* @param */
     ,0                      AS TaxAmount      /* @param */
)
INSERT INTO invoice_line_items (
	id
    ,invoice_id
    ,description
    ,quantity
    ,unit_amount
    ,line_amount
    ,account_code
    ,tax_amount
)
SELECT
    v.LineItemID 
    ,v.InvoiceID  
    ,v.Description
    ,v.Quantity   
    ,v.UnitAmount 
    ,v.LineAmount 
    ,v.AccountCode
    ,v.TaxAmount  
FROM
    variables v
;
