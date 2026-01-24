-- bank transactions holds Xero bank transactions.
CREATE TABLE bank_transactions (
    id                  TEXT PRIMARY KEY, -- Using TEXT for UUIDs is common in SQLite
    type                TEXT,
    status              TEXT,
    reference           TEXT,
    total               REAL,
    date                DATETIME,
    updated_at          DATETIME,
    contact_id          TEXT,
    contact_name        TEXT,
    /* bank account details for info only */
    bank_account_id     TEXT,
    bank_account_name   TEXT,
    bank_account_code   TEXT,
    /* reconciliation status relating to donations */
    is_reconciled       INTEGER DEFAULT 0 -- INTEGER 0 for false, 1 for true
);

-- Xero bank transaction line items.
CREATE TABLE bank_transaction_line_items (
    id              TEXT PRIMARY KEY,
    transaction_id  TEXT,
    description     TEXT,
    quantity        REAL,
    unit_amount     REAL,
    line_amount     REAL,
    account_code    TEXT, -- consider linking to accounts
    tax_amount      REAL,
    FOREIGN KEY(transaction_id) REFERENCES bank_transactions(id) ON DELETE CASCADE
);

-- Xero invoices.
CREATE TABLE invoices (
    id                  TEXT PRIMARY KEY,
    type                TEXT,
    status              TEXT,
    invoice_number      TEXT,
    reference           TEXT,
    total               REAL,
    amount_paid         REAL,
    date                DATETIME,
    updated_at          DATETIME,
    contact_id          TEXT,
    contact_name        TEXT,
    /* reconciliation status relating to donations */
    is_reconciled       INTEGER DEFAULT 0 -- INTEGER 0 for false, 1 for true
);

-- Xero invoice line items.
CREATE TABLE invoice_line_items (
    id              TEXT PRIMARY KEY,
    invoice_id      TEXT,
    description     TEXT,
    quantity        REAL,
    unit_amount     REAL,
    line_amount     REAL,
    account_code    TEXT, -- consider linking to accounts
    tax_amount      REAL,
    FOREIGN KEY(invoice_id) REFERENCES invoices(id) ON DELETE CASCADE
);

-- Xero accounts.
CREATE TABLE accounts (
   id             TEXT PRIMARY KEY,
   code           TEXT,
   name           TEXT,
   description    TEXT,
   type           TEXT,
   tax_type       TEXT,
   status         TEXT,
   system_account TEXT,
   currency_code  TEXT,
   updated_at     DATETIME
);

-- Salesforce opportunities are also known as "donations" when a charity
-- is using the Salesforce non-profit success pack (NPSP).
CREATE TABLE donations (
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
