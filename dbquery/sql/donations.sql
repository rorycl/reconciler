/*
 Reconciler app: SQL
 donations.sql
 list of donations with linkage status
 linkage status in this case only relates to whether the distributed foreign key
 (DFK) field payout_reference_dfk has a value or not.
 started: 02 January 2026

 Note: @ prefixed comments declare a template value for middleware replacement
*/

WITH concrete AS (
    SELECT
        date('2025-04-01') AS DateFrom             /* @DateFrom */
        ,date('2026-03-31') AS DateTo              /* @DateTo */
        -- All | Linked | NotLinked
        ,'Linked' AS LinkageStatus                 /* @LinkageStatus */
        ,'JG-PAYOUT-2025-04-15' AS PayoutReference /* @PayoutReference */
)

SELECT
    s.id  
    ,s.name 
    ,s.amount
    ,date(substring(s.close_date, 1, 10)) as close_date
    ,s.payout_reference_dfk
    ,date(substring(s.created_date, 1, 10)) as created_date
    ,s.created_by_name
    ,date(substring(s.last_modified_date, 1, 10)) as last_modified_date
    ,s.last_modified_by_name 
    /* see https://www.sqlitetutorial.net/sqlite-json-functions/sqlite-json_extract-function/ */
    -- s.additional_fields_json  TEXT -- A JSON blob for all other fields
FROM salesforce_opportunities s
JOIN concrete c ON s.close_date BETWEEN c.DateFrom AND c.DateTo
WHERE
    (c.LinkageStatus = 'All')
    OR
    (c.LinkageStatus = 'Linked'
        AND s.payout_reference_dfk IS NOT NULL
        AND (
            (c.PayoutReference is null)
            OR
            (c.PayoutReference = s.payout_reference_dfk)
        )
    )
    OR
    (c.LinkageStatus = 'NotLinked' AND s.payout_reference_dfk IS NULL)
;
