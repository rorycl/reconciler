INSERT INTO donations (
    id
    ,name
    ,amount
    ,close_date
    ,payout_reference_dfk
    ,created_date
    ,created_by_name
    ,last_modified_date
    ,last_modified_by_name
    ,additional_fields_json
)
VALUES (
    ?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?
    ,?)
ON CONFLICT (id) DO UPDATE SET
    name                    = excluded.name
    ,amount                 = excluded.amount
    ,close_date             = excluded.close_date
    ,payout_reference_dfk   = excluded.payout_reference_dfk
    ,created_date           = excluded.created_date
    ,created_by_name        = excluded.created_by_name
    ,last_modified_date     = excluded.last_modified_date
    ,last_modified_by_name  = excluded.last_modified_by_name
    ,additional_fields_json = excluded.additional_fields_json
;
