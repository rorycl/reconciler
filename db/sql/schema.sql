/*
 SQLite Schema file
 Tables for System, Xero and Salesforce data.
*/

CREATE TABLE IF NOT EXISTS system (
    id INTEGER PRIMARY KEY  -- Assuming only one row with id=1 for the desktop app

    -- oauth2 token information
    ,sf_token TEXT          -- marshalled oauth2.Token
    ,sf_instance_url TEXT
    ,xero_token TEXT        -- marshalled oauth2.Token
    ,xero_tenant_id TEXT

    -- data refresh metadata
    ,sf_data_refreshed DATETIME
    ,xero_data_refreshed DATETIME
);

-- Ensure only one row for applicaton state.
CREATE UNIQUE INDEX IF NOT EXISTS idx_single_row ON system ((1));

-- bank transactions holds Xero bank transactions.
CREATE TABLE IF NOT EXISTS bank_transactions (
    id                  TEXT PRIMARY KEY -- Use TEXT for UUIDs is common in SQLite
    ,type                TEXT
    ,status              TEXT
    ,reference           TEXT
    ,total               REAL
    ,date                DATETIME
    ,updated_at          DATETIME
    ,contact             TEXT
    ,bank_account        TEXT
    /* reconciliation status relating to donations */
    ,is_reconciled       INTEGER DEFAULT 0 -- INTEGER 0 for false 1 for true
);

-- Xero bank transaction line items.
CREATE TABLE IF NOT EXISTS bank_transaction_line_items (
    id              TEXT PRIMARY KEY
    ,transaction_id  TEXT
    ,description     TEXT
    ,quantity        REAL
    ,unit_amount     REAL
    ,line_amount     REAL
    ,account_code    TEXT -- consider linking to accounts
    ,tax_amount      REAL
    ,FOREIGN KEY(transaction_id) REFERENCES bank_transactions(id) ON DELETE CASCADE
);

-- Xero invoices.
CREATE TABLE IF NOT EXISTS invoices (
    id                  TEXT PRIMARY KEY
    ,type                TEXT
    ,status              TEXT
    ,invoice_number      TEXT
    ,reference           TEXT
    ,total               REAL
    ,amount_paid         REAL
    ,date                DATETIME
    ,updated_at          DATETIME
    ,contact             TEXT
    /* reconciliation status relating to donations */
    ,is_reconciled       INTEGER DEFAULT 0 -- INTEGER 0 for false 1 for true
);

-- Xero invoice line items.
CREATE TABLE IF NOT EXISTS invoice_line_items (
    id              TEXT PRIMARY KEY
    ,invoice_id      TEXT
    ,description     TEXT
    ,quantity        REAL
    ,unit_amount     REAL
    ,line_amount     REAL
    ,account_code    TEXT -- consider linking to accounts
    ,tax_amount      REAL
    ,FOREIGN KEY(invoice_id) REFERENCES invoices(id) ON DELETE CASCADE
);

-- Xero accounts.
CREATE TABLE IF NOT EXISTS accounts (
   id             TEXT PRIMARY KEY
   ,code           TEXT
   ,name           TEXT
   ,description    TEXT
   ,type           TEXT
   ,tax_type       TEXT
   ,status         TEXT
   ,system_account TEXT
   ,currency_code  TEXT
   ,updated_at     DATETIME
);

-- Salesforce opportunities are also known as "donations" when a charity
-- is using the Salesforce non-profit success pack (NPSP).
CREATE TABLE IF NOT EXISTS donations (
    id                      TEXT PRIMARY KEY
    ,name                    TEXT
    ,amount                  REAL
    ,close_date              DATETIME
    ,payout_reference_dfk    TEXT
    ,created_date            DATETIME
    ,created_by              TEXT
    ,last_modified_date      DATETIME
    ,last_modified_by        TEXT
    ,additional_fields_json  TEXT -- JSON blob for ancillary fields
);
