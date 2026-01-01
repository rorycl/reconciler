/*
 Reconciler app: SQL
 bank_transactions.sql
 list of bank transactions with reconciliation status
 started: 01 January 2026
*/

WITH concrete AS (
    SELECT
        date('2025-04-01') AS DateFrom
        ,date('2026-03-31') AS DateTo
        ,'^(53|55|57).*' AS AccountCodes
)

,bank_transaction_donation_totals AS (
    SELECT
        li.transaction_id
        ,SUM(li.line_amount) AS total_donation_amount
    FROM
        bank_transaction_line_items li
    JOIN 
        bank_transactions b ON (b.id = li.transaction_id)
        ,concrete
    WHERE
        account_code REGEXP concrete.AccountCodes
        AND
        b.status NOT IN ('AUTHORISED')
        AND
        date >= concrete.DateFrom AND date <= concrete.DateTo
    GROUP BY
        li.transaction_id
), 

crms_donation_totals AS (
    SELECT
        payout_reference_dfk
        ,SUM(amount) AS total_crms_amount
    FROM
        salesforce_opportunities
        ,concrete
    WHERE
        payout_reference_dfk IS NOT NULL
        AND
        close_date >= date(concrete.DateFrom,'-60 day')
        AND
        close_date <= date(concrete.DateTo, '+60 day')
    GROUP BY
        payout_reference_dfk
)

SELECT
    *
    ,CASE WHEN donation_total = crms_total
          THEN 1
          ELSE 0
          END AS reconciled
FROM
    (
    SELECT
        b.id
        ,b.reference
        ,date(substring(b.date, 1, 10)) AS date
        ,b.contact_name
        ,b.total
        ,COALESCE(idt.total_donation_amount, 0) AS donation_total
        ,COALESCE(cdt.total_crms_amount, 0) AS crms_total
    FROM
        bank_transactions b
        LEFT JOIN bank_transaction_donation_totals idt ON b.id = idt.transaction_id
        LEFT JOIN crms_donation_totals cdt ON b.reference = cdt.payout_reference_dfk
        ,concrete
    WHERE
        b.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        AND
        idt.transaction_id IS NOT NULL 
        AND
        b.date >= concrete.DateFrom AND b.date <= concrete.DateTo
) x
;
