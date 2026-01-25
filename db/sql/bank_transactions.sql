/*
 Reconciler app SQL
 bank_transactions.sql
 List view of bank transactions with reconciliation status.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
        date('2025-04-01') AS DateFrom   /* @param */
        ,date('2026-03-31') AS DateTo    /* @param */
        ,'^(53|55|57).*' AS AccountCodes /* @param */
        -- All | Reconciled | NotReconciled
        ,'NotReconciled' AS ReconciliationStatus   /* @param */
        ,'' AS TextSearch     /* @param */ 
        ,10 AS HereLimit                 /* @param */
        ,0 AS HereOffset                 /* @param */
)

,bank_transaction_donation_totals AS (
    SELECT
        li.transaction_id
        ,SUM(li.line_amount) AS total_donation_amount
    FROM bank_transaction_line_items li
    JOIN bank_transactions b ON (b.id = li.transaction_id)
    ,variables
    WHERE
        account_code REGEXP variables.AccountCodes
        AND
        b.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        AND
        b.date BETWEEN variables.DateFrom AND variables.DateTo
    GROUP BY
        li.transaction_id
),

crms_donation_totals AS (
    SELECT
        payout_reference_dfk
        ,SUM(amount) AS total_crms_amount
    FROM salesforce_opportunities
    JOIN variables
    WHERE
        payout_reference_dfk IS NOT NULL
        AND
        close_date BETWEEN date(variables.DateFrom,'-60 day') AND date(variables.DateTo, '+60 day')
    GROUP BY
        payout_reference_dfk
)

,reconciliation_data AS (
    SELECT
        b.id
        ,b.reference
        ,date
        ,b.contact_name
        ,b.status
        ,b.total
        ,COALESCE(bdt.total_donation_amount, 0) AS donation_total
        ,COALESCE(cdt.total_crms_amount, 0) AS crms_total
        ,COUNT(*) OVER () AS row_count
    FROM bank_transactions b
    JOIN variables v ON b.date BETWEEN v.DateFrom AND v.DateTo
    LEFT JOIN bank_transaction_donation_totals bdt ON b.id = bdt.transaction_id
    LEFT JOIN crms_donation_totals cdt ON b.reference = cdt.payout_reference_dfk
    WHERE
        b.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        AND
        b.date BETWEEN v.DateFrom AND v.DateTo
        AND (
            (v.ReconciliationStatus = 'All')
            OR
            (
                v.ReconciliationStatus = 'Reconciled'
                 AND 
                 COALESCE(bdt.total_donation_amount, 0) = COALESCE(cdt.total_crms_amount, 0)
            )
            OR
            (
                v.ReconciliationStatus = 'NotReconciled'
                 AND 
                 COALESCE(bdt.total_donation_amount, 0) <> COALESCE(cdt.total_crms_amount, 0)
            )
        )
        AND
        bdt.transaction_id IS NOT NULL 
        AND CASE
            WHEN v.TextSearch = '' THEN true
            ELSE LOWER(CONCAT(b.reference, ' ', b.contact_name)) REGEXP LOWER(v.TextSearch)
            END
    ORDER BY
        b.date ASC
)
SELECT
    r.*
    ,donation_total = crms_total AS is_reconciled
FROM reconciliation_data r
LIMIT
    (SELECT variables.HereLimit FROM variables)
OFFSET
    (SELECT variables.HereOffset FROM variables)
;
