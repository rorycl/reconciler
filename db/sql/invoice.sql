/*
 Reconciler app SQL
 invoice.sql
 Detail view of an invoice with line items and donation total.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
         'inv-unrec-04'  AS InvoiceID    /* @param */
        ,'^(53|55|57).*' AS AccountCodes /* @param */
)
SELECT
    *
    ,CASE WHEN donation_total = crms_total THEN
        1
     ELSE
        0
     END AS is_reconciled
    ,total - crms_total AS total_outstanding
FROM (
    SELECT
        i.id
        ,i.invoice_number
        ,i.date
        ,i.type
        ,i.status
        ,i.reference
        ,i.contact_name
        ,i.total
        ,COALESCE(
            SUM(li.line_amount) 
            FILTER (WHERE li.account_code REGEXP variables.AccountCodes)
            OVER (PARTITION BY i.id)
         , 0) AS donation_total
        ,COALESCE(rds.donation_sum, 0) AS crms_total
        -- line items
        -- Note that some line items only have a description, which
        -- works like a "note" in invoices and bank transactions.
        ,li.account_code AS li_account_code
        ,a.name AS account_name
        ,li.description AS li_description
        ,li.tax_amount AS li_tax_amount
        ,li.line_amount AS li_line_amount
        ,CASE WHEN
            li.account_code REGEXP variables.AccountCodes
        THEN
            li.line_amount
         ELSE
            0
         END AS li_donation_amount
    FROM
        invoices i
        ,variables
        LEFT OUTER JOIN invoice_line_items li ON (li.invoice_id = i.id)
        LEFT OUTER JOIN accounts a ON (li.account_code = a.code)
        -- reconciled_donations_summed rds is the total of
        -- salesforce_opportunites for this invoice.
        LEFT OUTER JOIN (
            SELECT
                payout_reference_dfk
                ,sum(amount) AS donation_sum
            FROM
                salesforce_opportunities
            GROUP BY
                payout_reference_dfk
        ) rds ON (rds.payout_reference_dfk = i.invoice_number)
    WHERE
        variables.InvoiceID = i.id
) x
;
