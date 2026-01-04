package db

// schema defines the SQL statements to create the application's database schema for SQLite.
const schema = `
CREATE TABLE IF NOT EXISTS salesforce_opportunities (
    id                      TEXT PRIMARY KEY,
    name                    TEXT,
    amount                  REAL,
    close_date              DATETIME,
    payout_reference_dfk    TEXT,
    created_date            DATETIME,
    created_by_name         TEXT,
    last_modified_date      DATETIME,
    last_modified_by_name   TEXT,
    additional_fields_json  TEXT -- A JSON blob for all other fields
);
`

// oppsUpsertSQL is the SQL statement for inserting or updating a Salesforce Opportunity in SQLite.
const oppsUpsertSQL = `
INSERT INTO salesforce_opportunities (
    id, name, amount, close_date, payout_reference_dfk,
    created_date, created_by_name, last_modified_date, last_modified_by_name,
    additional_fields_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    name = excluded.name,
    amount = excluded.amount,
    close_date = excluded.close_date,
    payout_reference_dfk = excluded.payout_reference_dfk,
    created_date = excluded.created_date,
    created_by_name = excluded.created_by_name,
    last_modified_date = excluded.last_modified_date,
    last_modified_by_name = excluded.last_modified_by_name,
    additional_fields_json = excluded.additional_fields_json;
`
