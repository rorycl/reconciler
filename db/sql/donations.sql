/*
 Reconciler app SQL
 donations.sql
 List of donations with linkage status linkage status in this case only
 relates to whether the distributed foreign key (DFK) is in both Xero
 invoices or bank transactions, and not just if the payout_reference_dfk
 has a value.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
        date('2025-04-01') AS DateFrom /* @param */
        ,date('2026-03-31') AS DateTo  /* @param */
        -- All | Linked | NotLinked
        ,'All' AS LinkageStatus        /* @param */
        ,'INV-2025-101' AS PayoutReference         /* @param */
        ,'' AS TextSearch              /* @param */
        ,30 AS HereLimit               /* @param */
        ,0 AS HereOffset               /* @param */
)

/* Although a salesforce opportunity ("donation") record with a filled
 * out payout_reference_dfk field suggests it is linked, that may not
 * always be the case due to (for example) deleted invoices or bank
 * transactions, or inaccurate data input, or related issues. This CTE
 * looks for valid records in the Xero invoices and bank
 * transactions to determine linkage (which is done in the `lit` LEFT
 * OUTER JOIN below.)
 */
,linked_invoices_or_transactions AS ( 
    SELECT
        i.invoice_number AS ref
    FROM
        invoices i
        ,variables v
    WHERE
        i.date BETWEEN date(v.DateFrom, '-60 day') AND date(v.DateTo, '+60 day')
        AND 
        i.invoice_number IS NOT NULL
        AND
        CASE
            WHEN PayoutReference = '' THEN
                TRUE
            ELSE
                i.invoice_number = PayoutReference
            END
    GROUP BY
        i.invoice_number

    UNION -- union the bank transactions to the invoices

    SELECT
        b.reference AS ref
    FROM
        bank_transactions b
        ,variables v
    WHERE
        b.date BETWEEN date(v.DateFrom, '-60 day') AND date(v.DateTo, '+60 day')
        AND 
        b.reference IS NOT NULL
        AND
        CASE
            WHEN PayoutReference = '' THEN
                TRUE
            ELSE
                b.reference = PayoutReference
            END
    GROUP BY
        b.reference
) 

,main AS (
    SELECT
        s.id  
        ,s.name 
        ,s.amount
        ,s.close_date
        ,s.payout_reference_dfk
        ,s.created_date
        ,s.created_by
        ,s.last_modified_date
        ,s.last_modified_by
        ,COUNT(*) OVER () AS row_count
        ,CASE
            WHEN lit.ref IS NOT NULL THEN
                TRUE
            ELSE
                FALSE
         END AS is_linked
        /* see www.sqlitetutorial.net/sqlite-json-functions/sqlite-json_extract-function/ */
        -- s.additional_fields_json  TEXT -- A JSON blob for all other fields
    FROM donations s
        LEFT OUTER JOIN linked_invoices_or_transactions lit ON (
            lit.ref = s.payout_reference_dfk
        )
        , variables v 
    WHERE
        s.close_date BETWEEN v.DateFrom AND v.DateTo
        AND
        CASE 
            -- Searching by v.PayoutReference doesn't make sense if
            -- v.LinkageStatus = 'NotLinked. If porting to plpgsql, check
            -- for that as an error condition in the preamble.
            WHEN v.LinkageStatus = 'NotLinked'
                AND v.PayoutReference IS NOT NULL
                AND v.PayoutReference <> '' THEN
                v.PayoutReference = s.payout_reference_dfk
            ELSE TRUE
        END
        AND
        (
            (v.LinkageStatus = 'All')
            OR
            (v.LinkageStatus = 'Linked' AND  lit.ref IS NOT NULL)
            OR
            (v.LinkageStatus = 'NotLinked' AND lit.ref IS NULL)
        )
        AND
        CASE
            WHEN v.TextSearch = '' OR v.TextSearch IS NULL THEN
                TRUE
            ELSE
                -- Todo searching the additional fields like this is very crude.
                -- LOWER(CONCAT(s.name, ' ', s.payout_reference_dfk)) REGEXP LOWER(v.TextSearch)
                LOWER(CONCAT(s.name, ' ', s.payout_reference_dfk, ' ', s.additional_fields_json)) REGEXP LOWER(v.TextSearch)
        END
        AND
        CASE
            WHEN v.PayoutReference = '' OR v.PayoutReference IS NULL THEN
                TRUE
            ELSE
                LOWER(s.payout_reference_dfk) = LOWER(v.PayoutReference)
        END
    ORDER BY
        s.close_date ASC
)

SELECT
    m.*
FROM main m
LIMIT
    (SELECT variables.HereLimit FROM variables)
OFFSET
    (SELECT variables.HereOffset FROM variables)
    
;
