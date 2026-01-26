/*
 Reconciler app SQL
 invoices.sql
 List of invoices with reconciliation status.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
        date('2025-04-01') AS DateFrom   /* @param */
        ,date('2026-03-31') AS DateTo    /* @param */
        ,'^(53|55|57).*' AS AccountCodes /* @param */
        -- All | Reconciled | NotReconciled
        ,'NotReconciled' AS ReconciliationStatus /* @param */
        ,'INV-2025.*Ex.*Corp' AS TextSearch      /* @param */
        ,10 AS HereLimit                         /* @param */
        ,0 AS HereOffset                         /* @param */
)

,invoice_donation_totals AS (
    SELECT
        li.invoice_id
        ,SUM(li.line_amount) AS total_donation_amount
    FROM invoice_line_items li
    JOIN invoices i ON (i.id = li.invoice_id)
    ,variables
    WHERE
        account_code REGEXP variables.AccountCodes
        AND
        i.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        AND
        i.date BETWEEN variables.DateFrom AND variables.DateTo
    GROUP BY
        li.invoice_id
),

crms_donation_totals AS (
    SELECT
        payout_reference_dfk
        ,SUM(amount) AS total_crms_amount
    FROM donations
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
        i.id
        ,i.invoice_number
        ,i.date
        ,i.contact
        ,i.status
        ,i.total
        ,COALESCE(idt.total_donation_amount, 0) AS donation_total
        ,COALESCE(cdt.total_crms_amount, 0) AS crms_total
        ,COUNT(*) OVER () AS row_count
    FROM invoices i
    JOIN variables v ON i.date BETWEEN v.DateFrom AND v.DateTo
    LEFT JOIN invoice_donation_totals idt ON i.id = idt.invoice_id
    LEFT JOIN crms_donation_totals cdt ON i.invoice_number = cdt.payout_reference_dfk
    WHERE
        i.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        AND
        i.date >= v.DateFrom AND i.date <= v.DateTo
        AND (
            (v.ReconciliationStatus = 'All')
            OR
            (
                v.ReconciliationStatus = 'Reconciled'
                 AND 
                 COALESCE(idt.total_donation_amount, 0) = COALESCE(cdt.total_crms_amount, 0)
            )
            OR
            (
                v.ReconciliationStatus = 'NotReconciled'
                 AND 
                 COALESCE(idt.total_donation_amount, 0) <> COALESCE(cdt.total_crms_amount, 0)
            )
        )
        AND
        idt.invoice_id IS NOT NULL
        AND CASE
            WHEN v.TextSearch = '' THEN true
            ELSE LOWER(CONCAT(i.invoice_number, ' ', i.reference, ' ', i.contact)) REGEXP LOWER(v.TextSearch)
            END
    ORDER BY
        i.date ASC
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
