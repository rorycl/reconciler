package db

// schema defines the SQL statements to create the application's database schema for SQLite.
// It is designed to be idempotent using `CREATE TABLE IF NOT EXISTS`.
const schema = "sql/schema.sql"

// btUpsertSQL is the SQL statement for inserting or updating a bank transaction in SQLite.
const btUpsertSQL = "sql/bank_transaction_upsert.sql"

// btLineDeleteSQL is the SQL statement for deleting all line items associated with a bank transaction.
const btLineDeleteSQL = "sql/bank_transaction_lis_delete.sql"

// btLineInsertSQL is the SQL statement for inserting a new bank transaction line item.
const btLineInsertSQL = "sql/bank_transaction_lis_insert.sql"

// invUpsertSQL is the SQL statement for inserting or updating an invoice in SQLite.
const invUpsertSQL = "sql/invoice_upsert.sql"

// invLineDeleteSQL is the SQL statement for deleting all line items associated with an invoice.
const invLineDeleteSQL = "sql/invoice_lis_delete.sql"

// invLineInsertSQL is the SQL statement for inserting a new invoice line item.
const invLineInsertSQL = "sql/invoice_lis_insert.sql"

// accUpsertSQL is the SQL statement for inserting or updating an account in SQLite.
const accUpsertSQL = "sql/accounts_upsert.sql"

// oppsUpsertSQL is the SQL statement for inserting or updating a Salesforce Opportunity in SQLite.
const oppsUpsertSQL = "sql/opportunity_upsert.sql"
