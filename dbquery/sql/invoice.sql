/*
 Reconciler app SQL
 invoice.sql
 detail of an invoice with line items and donation total
 started 01 January 2026

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

SELECT
    *
    ,CASE WHEN donation_total = crms_total THEN
        1
     ELSE
        0
     END AS is_reconciled
FROM (
    WITH variables AS (
        SELECT
             'inv-002' AS InvoiceID /* @param */
            ,'^(53|55|57).*' AS AccountCodes /* @param */
    )

    ,variables_extended AS (
        SELECT
             v.InvoiceID AS InvoiceID 
            ,v.AccountCodes AS AccountCodes 
            ,i.invoice_number AS InvoiceNumber
        FROM
            invoices i
            JOIN variables v ON (v.InvoiceID = i.id)
    )
    
    ,reconciled_donations_summed AS (
        SELECT
            payout_reference_dfk
            ,sum(amount) AS donation_sum
        FROM
            salesforce_opportunities
            ,variables_extended ve
        WHERE
            payout_reference_dfk = ve.InvoiceNumber
        GROUP BY
            payout_reference_dfk
    )
    SELECT
        i.id
        ,i.invoice_number
        ,i.date
        ,i.type
        ,i.status
        ,i.reference
        ,i.contact_name
        ,i.total
        ,sum(li.line_amount) 
            FILTER (WHERE li.account_code REGEXP variables.AccountCodes)
            OVER (PARTITION BY i.id) AS donation_total
        ,rds.donation_sum AS crms_total
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
        JOIN invoice_line_items li ON (li.invoice_id = i.id)
        LEFT OUTER JOIN accounts a ON (li.account_code = a.code)
        LEFT OUTER JOIN reconciled_donations_summed rds ON (rds.payout_reference_dfk = i.invoice_number)
    WHERE
        variables.InvoiceID = i.id
) x
;
