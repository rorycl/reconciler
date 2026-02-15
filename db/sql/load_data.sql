-- =============================================================================
-- Test Data for Xero-Salesforce Reconciliation App
--
-- Financial Year Start: April 1st
-- Fictional "Today": ~ May 15th, 2025
-- Current Financial Year for testing: 2025/2026 (starts 2025-04-01)
--
-- Note that salesforce "opportunities" are referred to as "donations".
-- =============================================================================

-- Make script re-runnable by deleting existing data.
DELETE FROM donations;
DELETE FROM invoice_line_items;
DELETE FROM invoices;
DELETE FROM bank_transaction_line_items;
DELETE FROM bank_transactions;
DELETE FROM accounts;

PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;

-- -----------------------------------------------------------------------------
-- Accounts
-- donation and platform fee accounts
-- -----------------------------------------------------------------------------

INSERT INTO "accounts" (id, code, name, description, type, status) VALUES
('acc-5301', '5301', 'Fundraising Dinners', 'Income from fundraising dinner events', 'REVENUE', 'ACTIVE'),
('acc-5501', '5501', 'General Giving', 'Unrestricted donation income', 'REVENUE', 'ACTIVE'),
('acc-5701', '5701', 'Spring Campaign 2025', 'Restricted income for the Spring 2025 Campaign', 'REVENUE', 'ACTIVE'),
('acc-429', '429', 'Platform Fees', 'Fees deducted by payment processors like Stripe, JustGiving', 'EXPENSE', 'ACTIVE'),
('acc-9999', '9999', 'Arbitrary', 'Arbitrary accounts', 'LIABILITY', 'ACTIVE')
;

-- -----------------------------------------------------------------------------
-- Invoice scenario 1
-- A simple fully reconciled invoice 
-- * a single invoice line item (no platform fees) 
-- * a single salesforce donation
-- -----------------------------------------------------------------------------
INSERT INTO "invoices" (id, invoice_number, status, total, date, contact) VALUES
('inv-001', 'INV-2025-101', 'PAID', 500.00, '2025-04-10T10:00:00Z', 'Example Corp Ltd');
INSERT INTO "invoice_line_items" (id, invoice_id, description, line_amount, account_code) VALUES
('inv-li-001', 'inv-001', 'Donation for Q1 2025', 500.00, '5501');

INSERT INTO "donations" (id, name, amount, close_date, payout_reference_dfk) VALUES
('sf-opp-001', 'Example Corp Q1 Donation', 500.00, datetime('2025-04-08'), 'INV-2025-101');

-- -----------------------------------------------------------------------------
-- Invoice scenario 2
-- A fully reconciled invoice
-- * two invoice line items (donation and fee)
-- * one SF Opportunity.
-- the SF donation amount should match the gross donation, not the invoice total.
-- -----------------------------------------------------------------------------
INSERT INTO "invoices" (id, invoice_number, status, total, date, contact) VALUES
('inv-002', 'INV-2025-102', 'PAID', 196.50, '2025-04-12T11:00:00Z', 'Generous Individual');
INSERT INTO "invoice_line_items" (id, invoice_id, description, line_amount, account_code) VALUES
('inv-li-002a', 'inv-002', 'Pledged donation via Stripe', 200.00, '5301'),
('inv-li-002b', 'inv-002', 'Stripe processing fee', -3.50, '429');

INSERT INTO "donations" (id, name, amount, close_date, payout_reference_dfk) VALUES
('sf-opp-002', 'Generous Individual Pledge', 200.00, datetime('2025-04-11'), 'INV-2025-102');

-- -----------------------------------------------------------------------------
-- Invoice scenario 3
-- Unreconciled items in the current financial year
-- -----------------------------------------------------------------------------
INSERT INTO "invoices" (id, invoice_number, status, total, date, contact) VALUES
('inv-unrec-01', 'INV-2025-103', 'PAID', 1000.00, '2025-04-16T10:00:00Z', 'Another Corp'),
('inv-unrec-02', 'INV-2025-104', 'PAID', 250.00, '2025-04-18T11:00:00Z', 'Local Business Ltd'),
('inv-unrec-03', 'INV-2025-105', 'PAID', 750.00, '2025-04-21T12:00:00Z', 'Community Fund'),
('inv-unrec-04', 'INV-2025-106', 'PAID', 50.00, '2025-04-25T13:00:00Z', 'Small Pledge'),
('inv-unrec-05', 'INV-2025-107', 'PAID', 300.00, '2025-05-02T14:00:00Z', 'Grant Giver'),
('inv-unrec-06', 'INV-2025-108', 'PAID', 2000.00, '2025-05-05T15:00:00Z', 'Major Donor Pledge');
INSERT INTO "invoice_line_items" (id, invoice_id, description, line_amount, account_code) VALUES
('inv-li-unrec-01', 'inv-unrec-01', 'Corporate Partnership Donation', 1000.00, '5301'),
('inv-li-unrec-02', 'inv-unrec-02', 'Sponsorship Donation', 250.00, '5301'),
('inv-li-unrec-03', 'inv-unrec-03', 'Donation', 750.00, '5501'),
('inv-li-unrec-04', 'inv-unrec-04', 'Donation', 50.00, '5501'),
('inv-li-unrec-05', 'inv-unrec-05', 'Donation', 300.00, '5501'),
('inv-li-unrec-06', 'inv-unrec-06', 'Donation', 2000.00, '5501');

-- -----------------------------------------------------------------------------
-- Invoice scenario 4
-- Items from the previous financial year to test data filtering
-- -----------------------------------------------------------------------------
INSERT INTO "invoices" (id, invoice_number, status, total, date, contact) VALUES
('inv-prev-fy-01', 'INV-2024-950', 'PAID', 150.00, '2025-03-25T10:00:00Z', 'Old Pledge Inc.');
INSERT INTO "invoice_line_items" (id, invoice_id, description, line_amount, account_code) VALUES
('inv-li-prev-fy-01', 'inv-prev-fy-01', 'End of Year Donation', 150.00, '5501');

-- -----------------------------------------------------------------------------
-- Invoice scenario 5
-- Arbitrary invoices that have nothing to do with donations
-- -----------------------------------------------------------------------------
INSERT INTO "invoices" (id, invoice_number, status, total, date, contact) VALUES
('inv-arb-01', 'INV-2025-110', 'DRAFT', 1.00, '2025-07-01T00:00:00Z', 'Future Invoices Inc.');
INSERT INTO "invoice_line_items" (id, invoice_id, description, line_amount, account_code) VALUES
('inv-li-arb-01-01', 'inv-arb-01', 'An arbitrary entry', 1.00, '9999');

INSERT INTO "invoices" (id, invoice_number, status, total, date, contact) VALUES
('inv-arb-02', 'INV-2025-111', 'AUTHORISED', 2.00, '2025-07-02T00:00:00Z', 'Future Invoices Inc.');
INSERT INTO "invoice_line_items" (id, invoice_id, description, line_amount, account_code) VALUES
('inv-li-arb-02-01', 'inv-arb-02', 'Another arbitrary entry', 2.00, '9999');

-- -----------------------------------------------------------------------------
-- Bank Transaction scenario 1
-- A fully reconciled bank transaction for pagination testing.
-- represents a weekly payout from a platform like JustGiving.
-- * linked to > 10 sf donations.
-- * income split across multiple donation accounts.
-- * platform fee deducted.
-- gross donations: 12 donations totaling 355.00
-- platform fee: 5% of gross = 17.75
-- net payout (bank transaction total): 337.25
-- -----------------------------------------------------------------------------
INSERT INTO "bank_transactions" (id, reference, status, total, date, contact, bank_account_id) VALUES
('bt-001', 'JG-PAYOUT-2025-04-15', 'RECONCILED', 337.25, '2025-04-15T14:00:00Z', 'JustGiving', '7404f143aa1c');
INSERT INTO "bank_transaction_line_items" (id, transaction_id, description, line_amount, account_code) VALUES
('bt-li-001a', 'bt-001', 'JustGiving Payout - General Giving', 200.00, '5501'),
('bt-li-001b', 'bt-001', 'JustGiving Payout - Spring Campaign', 155.00, '5701'),
('bt-li-001c', 'bt-001', 'JustGiving Platform Fee', -17.75, '429');

INSERT INTO "donations" (id, name, amount, close_date, payout_reference_dfk) VALUES
('sf-opp-003', 'Anonymous Donor', 20.00, datetime('2025-04-13'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-004', 'Anonymous Donor', 20.00, datetime('2025-04-13'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-005', 'Jane Smith',     100.00, datetime('2025-04-14'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-006', 'Anonymous Donor', 20.00, datetime('2025-04-14'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-007', 'Anonymous Donor', 20.00, datetime('2025-04-14'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-008', 'Anonymous Donor', 20.00, datetime('2025-04-14'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-009', 'Anonymous Donor', 20.00, datetime('2025-04-14'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-010', 'Anonymous Donor', 20.00, datetime('2025-04-14'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-011', 'Anonymous Donor', 20.00, datetime('2025-04-14'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-012', 'Anonymous Donor', 20.00, datetime('2025-04-14'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-013', 'John Doe', 55.00, datetime('2025-04-15'), 'JG-PAYOUT-2025-04-15'),
('sf-opp-014', 'Anonymous Donor', 20.00, datetime('2025-04-15'), 'JG-PAYOUT-2025-04-15');

-- -----------------------------------------------------------------------------
-- Bank Transaction scenario 2
-- A partially reconciled bank transaction.
-- donation income is 500, but only 250 is accounted for in linked sf opps.
-- -----------------------------------------------------------------------------
INSERT INTO "bank_transactions" (id, reference, status, total, date, contact, bank_account_id) VALUES
('bt-002', 'STRIPE-PAYOUT-2025-04-20', 'RECONCILED', 490.00, '2025-04-20T09:00:00Z', 'Stripe', 'ee898997-09f4-11f1-a10c-7404f143aa1c');
INSERT INTO "bank_transaction_line_items" (id, transaction_id, description, line_amount, account_code) VALUES
('bt-li-002a', 'bt-002', 'Stripe Payout', 500.00, '5501'),
('bt-li-002b', 'bt-002', 'Stripe Platform Fee', -10.00, '429');

-- reconciled
INSERT INTO "donations" (id, name, amount, close_date, payout_reference_dfk) VALUES
('sf-opp-015', 'Online Donation', 100.00, datetime('2025-04-18'), 'STRIPE-PAYOUT-2025-04-20'),
('sf-opp-016', 'Social Media Donation', 150.00, datetime('2025-04-19'), 'STRIPE-PAYOUT-2025-04-20');

-- not reconciled
INSERT INTO "donations" (id, name, amount, close_date, payout_reference_dfk) VALUES
('sf-opp-017', 'Online Donation 2', 150.00, datetime('2025-04-16'), null),
('sf-opp-018', 'Online Donation 3', 50.00, datetime('2025-04-17'), null),
('sf-opp-019', 'Social Media Donation 2', 50.00, datetime('2025-04-15'), null);

-- -----------------------------------------------------------------------------
-- Bank Transaction scenario 3
-- Unreconciled items in the current financial year
-- -----------------------------------------------------------------------------
-- 6 Unreconciled Bank Transactions
INSERT INTO "bank_transactions" (id, reference, status, total, date, contact, bank_account_id) VALUES
('bt-unrec-01', 'JG-PAYOUT-2025-04-22'     , 'RECONCILED', 97.50 , '2025-04-22T14:00:00Z', 'JustGiving'              , '7404f143aa1a'),
('bt-unrec-02', 'STRIPE-PAYOUT-2025-04-27' , 'RECONCILED', 245.00, '2025-04-27T09:00:00Z', 'Stripe'                  , '7404f143aa1b'),
('bt-unrec-03', 'ENTHUSE-PAYOUT-2025-04-28', 'RECONCILED', 112.00, '2025-04-28T10:00:00Z', 'Enthuse'                 , '7404f143aa1c'),
('bt-unrec-04', 'JG-PAYOUT-2025-04-29'     , 'RECONCILED', 146.25, '2025-04-29T14:00:00Z', 'JustGiving'              , '7404f143aa1d'),
('bt-unrec-05', 'CAF-PAYOUT-2025-05-01'    , 'RECONCILED', 500.00, '2025-05-01T11:00:00Z', 'Charities Aid Foundation', '7404f143aa1e'),
('bt-unrec-06', 'STRIPE-PAYOUT-2025-05-04' , 'RECONCILED', 332.50, '2025-05-04T09:00:00Z', 'Stripe'                  , '7404f143aa1f');
INSERT INTO "bank_transaction_line_items" (id, transaction_id, description, line_amount, account_code) VALUES
('bt-li-unrec-01a', 'bt-unrec-01', 'Donation Payout', 100.00, '5501'), ('bt-li-unrec-01b', 'bt-unrec-01', 'Fee', -2.50, '429'),
('bt-li-unrec-02a', 'bt-unrec-02', 'Donation Payout', 250.00, '5701'), ('bt-li-unrec-02b', 'bt-unrec-02', 'Fee', -5.00, '429'),
('bt-li-unrec-03a', 'bt-unrec-03', 'Donation Payout', 115.00, '5501'), ('bt-li-unrec-03b', 'bt-unrec-03', 'Fee', -3.00, '429'),
('bt-li-unrec-04a', 'bt-unrec-04', 'Donation Payout', 150.00, '5501'), ('bt-li-unrec-04b', 'bt-unrec-04', 'Fee', -3.75, '429'),
('bt-li-unrec-05a', 'bt-unrec-05', 'Donation Payout', 500.00, '5501'),
('bt-li-unrec-06a', 'bt-unrec-06', 'Donation Payout', 340.00, '5701'), ('bt-li-unrec-06b', 'bt-unrec-06', 'Fee', -7.50, '429');

-- -----------------------------------------------------------------------------
-- Bank Transaction scenario 4
-- Items from the previous financial year
-- -----------------------------------------------------------------------------
-- A reconciled bank transaction from Feb 2025
INSERT INTO "bank_transactions" (id, reference, status, total, date, contact, bank_account_id) VALUES
('bt-prev-fy-01', 'JG-PAYOUT-2025-02-28', 'RECONCILED', 190.00, '2025-02-28T14:00:00Z', 'JustGiving', '7404f143aa1c');
INSERT INTO "bank_transaction_line_items" (id, transaction_id, description, line_amount, account_code) VALUES
('bt-li-prev-fy-01a', 'bt-prev-fy-01', 'Donation Payout', 200.00, '5501'),
('bt-li-prev-fy-01b', 'bt-prev-fy-01', 'Fee', -10.00, '429');
INSERT INTO "donations" (id, name, amount, close_date, payout_reference_dfk) VALUES
('sf-opp-prev-fy-01', 'Old Donation', 200.00, datetime('2025-02-26'), 'JG-PAYOUT-2025-02-28');

-- -----------------------------------------------------------------------------
-- Salesforce scenario 1
-- data oddities
-- -----------------------------------------------------------------------------
-- An donation with a "bad" date (date after the payout date)
INSERT INTO "donations" (id, name, amount, close_date, payout_reference_dfk) VALUES
('sf-opp-odd-01', 'Data Entry Error Donation', 50.00, datetime('2025-06-10'), 'INV-2025-101');

-- An unlinked donation 
INSERT INTO "donations" (id, name, amount, close_date, payout_reference_dfk) VALUES
('sf-opp-odd-02', 'Unlinked Donation', 75.00, datetime('2025-04-30'), null);

COMMIT;
PRAGMA foreign_keys=ON;
