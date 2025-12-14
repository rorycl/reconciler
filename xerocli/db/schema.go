package db

// schema defines the SQL statements to create the application's database schema.
// It is designed to be idempotent using `CREATE TABLE IF NOT EXISTS`.
const schema = `
CREATE TABLE IF NOT EXISTS bank_transactions (
    id                  UUID PRIMARY KEY,
    type                VARCHAR,
    status              VARCHAR,
    reference           VARCHAR,
    total               DECIMAL(18, 2),
    is_reconciled       BOOLEAN,
    date                TIMESTAMP,
    updated_at          TIMESTAMP,
    contact_id          UUID,
    contact_name        VARCHAR,
    bank_account_id     UUID,
    bank_account_name   VARCHAR,
    bank_account_code   VARCHAR
);

CREATE TABLE IF NOT EXISTS bank_transaction_line_items (
    id              UUID PRIMARY KEY,
    transaction_id  UUID,
    description     VARCHAR,
    quantity        DECIMAL(18, 4),
    unit_amount     DECIMAL(18, 4),
    line_amount     DECIMAL(18, 2),
    account_code    VARCHAR,
    tax_amount      DECIMAL(18, 2)
);

CREATE TABLE IF NOT EXISTS invoices (
    id                  UUID PRIMARY KEY,
    type                VARCHAR,
    status              VARCHAR,
    invoice_number      VARCHAR,
    reference           VARCHAR,
    total               DECIMAL(18, 2),
    amount_paid         DECIMAL(18, 2),
    date                TIMESTAMP,
    updated_at          TIMESTAMP,
    contact_id          UUID,
    contact_name        VARCHAR
);

CREATE TABLE IF NOT EXISTS invoice_line_items (
    id              UUID PRIMARY KEY,
    invoice_id      UUID,
    description     VARCHAR,
    quantity        DECIMAL(18, 4),
    unit_amount     DECIMAL(18, 4),
    line_amount     DECIMAL(18, 2),
    account_code    VARCHAR,
    tax_amount      DECIMAL(18, 2)
);
`

// btUpsertSQL is the SQL statement for inserting or updating a bank transaction.
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

// invUpsertSQL is the SQL statement for inserting or updating an invoice.
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
