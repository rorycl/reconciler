/*
 Reconciler app SQL
 organisation_upsert.sql
 Upsert a Xero Organisation into the database.

 Note @param comments declare a template value for middleware replacement.
 Note do _not_ use colons in sql or comments as it breaks the sqlx parser.
*/

WITH variables AS (
    SELECT
         '1'                       AS ID  -- not a param
         ,'Demo Co'                AS Name                  /* @param */
         ,'Demo Co Ltd'            AS LegalName             /* @param */
         ,'Charity'                AS OrganisationType      /* @param */
         ,'15'                     AS FinancialYearEndDay   /* @param */
         ,'4'                      AS FinancialYearEndMonth /* @param */
         ,'NEWZEALANDSTANDARDTIME' AS Timezone              /* @param */
         ,'!NxTp!'                 AS ShortCode             /* @param */
         ,'7404f143aa1c'           AS OrganisationID        /* @param */
)

INSERT INTO organisation (
    id
    ,name
    ,legal_name
    ,organisation_type
    ,financial_year_end_day
    ,financial_year_end_month
    ,timezone
    ,shortcode
    ,organisation_id
)
SELECT
    v.ID  -- not a param
    ,v.Name                 
    ,v.LegalName            
    ,v.OrganisationType     
    ,v.FinancialYearEndDay  
    ,v.FinancialYearEndMonth
    ,v.Timezone             
    ,v.ShortCode            
    ,v.OrganisationID       
FROM
    variables v
-- sqlite.org/lang_upsert.html PARSING AMBIGUITY
WHERE
    true
ON CONFLICT (id) DO UPDATE SET
    id                        = excluded.id                               
    ,name                     = excluded.name
    ,legal_name               = excluded.legal_name
    ,organisation_type        = excluded.organisation_type
    ,financial_year_end_day   = excluded.financial_year_end_day
    ,financial_year_end_month = excluded.financial_year_end_month
    ,timezone                 = excluded.timezone
    ,shortcode                = excluded.shortcode
    ,organisation_id          = excluded.organisation_id
;
