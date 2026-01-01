/*
 Reconciler app: SQL
 invoices.sql
 list of invoices with reconciliation status
 started: 01 January 2026
*/

WITH concrete AS (
    SELECT
        date('2025-04-01') AS DateFrom
        ,date('2026-03-31') AS DateTo
)

,invoice_donation_totals AS (
  SELECT
    li.invoice_id
    ,SUM(li.line_amount) AS total_donation_amount
  FROM
    invoice_line_items li
    JOIN invoices i ON (i.id = li.invoice_id)
    ,concrete
  WHERE
    (
        account_code LIKE '53%' OR
        account_code LIKE '55%' OR
        account_code LIKE '57%'
    ) AND
    i.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
    AND
    date >= concrete.DateFrom AND date <= concrete.DateTo
  GROUP BY
    li.invoice_id
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
        close_date >= date(concrete.DateFrom,'-14 day')
        AND close_date <= date(concrete.DateTo, '+14 day')
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
        i.id
        ,i.invoice_number
        ,date(substring(i.date, 1, 10)) AS date
        ,i.contact_name
        ,i.total
        ,COALESCE(idt.total_donation_amount, 0) AS donation_total
        ,COALESCE(cdt.total_crms_amount, 0) AS crms_total
    FROM
        invoices i
        LEFT JOIN invoice_donation_totals idt ON i.id = idt.invoice_id
        LEFT JOIN crms_donation_totals cdt ON i.invoice_number = cdt.payout_reference_dfk
        ,concrete
    WHERE
        i.status NOT IN ('DRAFT', 'DELETED', 'VOIDED')
        AND idt.invoice_id IS NOT NULL 
        AND
        i.date >= concrete.DateFrom AND i.date <= concrete.DateTo
) x
;
