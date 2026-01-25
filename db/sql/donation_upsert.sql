/*
 Reconciler app SQL
 donation_upsert.sql 
 Upsert a donation (salesforce opportunity) record.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
        'sf-opp-003'            AS ID                   /* @param */
        ,'Anonymous Donor'      AS Name                 /* @param */
        ,'21.20'                AS Amount               /* @param */
        ,datetime('2025-04-14') AS CloseDate            /* @param */
        ,'JG-PAYOUT-2025-04-15' AS PayoutReference      /* @param */
        ,datetime('2025-04-01') AS CreatedDate          /* @param */
        ,'User1'                AS CreatedBy            /* @param */
        ,datetime('2025-04-01') AS LastModifiedDate     /* @param */
        ,'User1'                AS LastModifiedBy       /* @param */
        ,''                     AS AdditionalFieldsJSON /* @param */
)
INSERT INTO donations (
    id
    ,name
    ,amount
    ,close_date
    ,payout_reference_dfk
    ,created_date
    ,created_by
    ,last_modified_date
    ,last_modified_by
    ,additional_fields_json
)
SELECT
    v.ID
    ,v.Name
    ,v.Amount
    ,v.CloseDate
    ,v.PayoutReference
    ,v.CreatedDate
    ,v.CreatedBy
    ,v.LastModifiedDate
    ,v.LastModifiedBy
    ,v.AdditionalFieldsJSON
FROM
    variables v
-- sqlite.org/lang_upsert.html PARSING AMBIGUITY
WHERE
    true
ON CONFLICT (id) DO UPDATE SET
    name                    = excluded.name
    ,amount                 = excluded.amount
    ,close_date             = excluded.close_date
    ,payout_reference_dfk   = excluded.payout_reference_dfk
    ,created_date           = excluded.created_date
    ,created_by             = excluded.created_by
    ,last_modified_date     = excluded.last_modified_date
    ,last_modified_by       = excluded.last_modified_by
    ,additional_fields_json = excluded.additional_fields_json
;
