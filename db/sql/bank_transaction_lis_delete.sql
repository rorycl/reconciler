/*
 Reconciler app SQL
 bank_transaction_lis_delete.sql
 Delete bank transaction line items by transaction_id

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
         'bt-unrec-01' AS BankTransactionID /* @param */
)
DELETE FROM
    bank_transaction_line_items
WHERE
    transaction_id = (
        SELECT BankTransactionID from variables
    )
;
