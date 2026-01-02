/*
 Reconciler app: SQL
 bank_transaction.sql
 detail of a bank transaction with line items and donation total
 started: 01 January 2026

 Note: @ prefixed comments declare a template value for middleware replacement
*/

SELECT
    *
    ,CASE WHEN donation_total = crms_total THEN
        1
     ELSE
        0
     END AS reconciled
FROM (
    WITH concrete AS (
        SELECT
             'JG-PAYOUT-2025-02-28' AS BankTransactionReference /* @BankTransactionReference */
            ,'^(53|55|57).*' AS AccountCodes                    /* @AccountCodes */
    )
    
    ,reconciled_donations_summed AS (
        SELECT
            payout_reference_dfk
            ,sum(amount) AS donation_sum
        FROM
            salesforce_opportunities
            ,concrete
        WHERE
            payout_reference_dfk = concrete.BankTransactionReference
        GROUP BY
            payout_reference_dfk
    )

    SELECT
        b.id
        ,b.reference
        ,date(substring(b.date, 1, 10)) AS date
        ,b.type
        ,b.status
        ,b.reference
        ,b.contact_name
        ,b.total
        ,sum(li.line_amount) 
            FILTER (WHERE li.account_code REGEXP concrete.AccountCodes)
            OVER (PARTITION BY b.id) AS donation_total
        ,rds.donation_sum AS crms_total
        ,a.name AS account_name
        ,li.description AS li_description
        ,li.tax_amount AS li_tax_amount
        ,li.line_amount AS li_line_amount
        ,CASE WHEN
            li.account_code REGEXP concrete.AccountCodes
        THEN
            li.line_amount
         ELSE
            0
         END AS li_donation_amount
    FROM
        bank_transactions b
        JOIN bank_transaction_line_items li ON (li.transaction_id = b.id)
        LEFT OUTER JOIN accounts a ON (li.account_code = a.code)
        LEFT OUTER JOIN reconciled_donations_summed rds ON (rds.payout_reference_dfk = b.reference)
        ,concrete
    WHERE
        b.reference = concrete.BankTransactionReference 
        /*
        AND
        b.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        */
) x
;
