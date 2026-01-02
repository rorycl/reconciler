/*
 Reconciler app: SQL
 bank_transactions.sql
 list of bank transactions with reconciliation status
 started: 01 January 2026

 Note: @ prefixed comments declare a template value for middleware replacement
*/

WITH concrete AS (
    SELECT
        date('2025-04-01') AS DateFrom   /* @DateFrom */
        ,date('2026-03-31') AS DateTo    /* @DateTo */
        ,'^(53|55|57).*' AS AccountCodes /* @AccountCodes */
        -- All | Reconciled | NotReconciled
        ,'All' AS ReconciliationStatus   /* @ReconciliationStatus */
)

,bank_transaction_donation_totals AS (
    SELECT
        li.transaction_id
        ,SUM(li.line_amount) AS total_donation_amount
    FROM bank_transaction_line_items li
    JOIN bank_transactions b ON (b.id = li.transaction_id)
    ,concrete
    WHERE
        account_code REGEXP concrete.AccountCodes
        AND
        b.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        AND
        date BETWEEN concrete.DateFrom AND concrete.DateTo
    GROUP BY
        li.transaction_id
), 

crms_donation_totals AS (
    SELECT
        payout_reference_dfk
        ,SUM(amount) AS total_crms_amount
    FROM salesforce_opportunities
    JOIN concrete
    WHERE
        payout_reference_dfk IS NOT NULL
        AND
        close_date BETWEEN date(concrete.DateFrom,'-60 day') AND date(concrete.DateTo, '+60 day')
    GROUP BY
        payout_reference_dfk
)

,reconciliation_data AS (
    SELECT
        b.id,
        b.reference,
        date(substring(b.date, 1, 10)) AS date,
        b.contact_name,
        b.total,
        COALESCE(bdt.total_donation_amount, 0) AS donation_total,
        COALESCE(cdt.total_crms_amount, 0) AS crms_total
    FROM bank_transactions b
    JOIN concrete c ON b.date BETWEEN c.DateFrom AND c.DateTo
    LEFT JOIN bank_transaction_donation_totals bdt ON b.id = bdt.transaction_id
    LEFT JOIN crms_donation_totals cdt ON b.reference = cdt.payout_reference_dfk
    WHERE
        b.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        AND bdt.transaction_id IS NOT NULL 
)
SELECT 
    r.*,
    (donation_total = crms_total) AS is_reconciled
FROM 
    reconciliation_data r
JOIN concrete c
WHERE
    (c.ReconciliationStatus = 'All')
    OR
    (c.ReconciliationStatus = 'Reconciled' AND r.donation_total = r.crms_total)
    OR
    (c.ReconciliationStatus = 'NotReconciled' AND r.donation_total <> r.crms_total);
;
