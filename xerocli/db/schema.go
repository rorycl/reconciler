package db

// schema defines the SQL statements to create the application's database schema for SQLite.
// It is designed to be idempotent using `CREATE TABLE IF NOT EXISTS`.
const schema = `
CREATE TABLE IF NOT EXISTS bank_transactions (
    id                  TEXT PRIMARY KEY, -- Using TEXT for UUIDs is common in SQLite
    type                TEXT,
    status              TEXT,
    reference           TEXT,
    total               REAL,
    is_reconciled       INTEGER, -- INTEGER 0 for false, 1 for true
    date                DATETIME,
    updated_at          DATETIME,
    contact_id          TEXT,
    contact_name        TEXT,
    bank_account_id     TEXT,
    bank_account_name   TEXT,
    bank_account_code   TEXT
);

CREATE TABLE IF NOT EXISTS bank_transaction_line_items (
    id              TEXT PRIMARY KEY,
    transaction_id  TEXT,
    description     TEXT,
    quantity        REAL,
    unit_amount     REAL,
    line_amount     REAL,
    account_code    TEXT,
    tax_amount      REAL,
    FOREIGN KEY(transaction_id) REFERENCES bank_transactions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS invoices (
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
    contact_name        TEXT
);

CREATE TABLE IF NOT EXISTS invoice_line_items (
    id              TEXT PRIMARY KEY,
    invoice_id      TEXT,
    description     TEXT,
    quantity        REAL,
    unit_amount     REAL,
    line_amount     REAL,
    account_code    TEXT,
    tax_amount      REAL,
    FOREIGN KEY(invoice_id) REFERENCES invoices(id) ON DELETE CASCADE
);
`

// btUpsertSQL is the SQL statement for inserting or updating a bank transaction in SQLite.
// SQLite uses `INSERT ... ON CONFLICT ... DO UPDATE`.
const btUpsertSQL = `
INSERT INTO bank_transactions (id, type, status, reference, total, is_reconciled, date, updated_at, contact_id, contact_name, bank_account_id, bank_account_name, bank_account_code)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    type = excluded.type,
    status = excluded.status,
    reference = excluded.reference,
    total = excluded.total,
    is_reconciled = excluded.is_reconciled,
    date = excluded.date,
    updated_at = excluded.updated_at,
    contact_id = excluded.contact_id,
    contact_name = excluded.contact_name,
    bank_account_id = excluded.bank_account_id,
    bank_account_name = excluded.bank_account_name,
    bank_account_code = excluded.bank_account_code;
`

// btLineDeleteSQL is the SQL statement for deleting all line items associated with a bank transaction.
const btLineDeleteSQL = `DELETE FROM bank_transaction_line_items WHERE transaction_id = ?;`

// btLineInsertSQL is the SQL statement for inserting a new bank transaction line item.
const btLineInsertSQL = `
INSERT INTO bank_transaction_line_items (id, transaction_id, description, quantity, unit_amount, line_amount, account_code, tax_amount)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`

// invUpsertSQL is the SQL statement for inserting or updating an invoice in SQLite.
const invUpsertSQL = `
INSERT INTO invoices (id, type, status, invoice_number, reference, total, amount_paid, date, updated_at, contact_id, contact_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    type = excluded.type,
    status = excluded.status,
    invoice_number = excluded.invoice_number,
    reference = excluded.reference,
    total = excluded.total,
    amount_paid = excluded.amount_paid,
    date = excluded.date,
    updated_at = excluded.updated_at,
    contact_id = excluded.contact_id,
    contact_name = excluded.contact_name;
`

// invLineDeleteSQL is the SQL statement for deleting all line items associated with an invoice.
const invLineDeleteSQL = `DELETE FROM invoice_line_items WHERE invoice_id = ?;`

// invLineInsertSQL is the SQL statement for inserting a new invoice line item.
const invLineInsertSQL = `
INSERT INTO invoice_line_items (id, invoice_id, description, quantity, unit_amount, line_amount, account_code, tax_amount)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`
