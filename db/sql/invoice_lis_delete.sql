/*
 Reconciler app SQL
 invoice_lis_delete.sql
 Delete invoice line items by invoice_id.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
         'inv-002' AS InvoiceID /* @param */
)
DELETE FROM 
    invoice_line_items
WHERE
    invoice_id = (
        SELECT InvoiceID from variables
    )
;
