# xerocli

Work in Progress.

* Login to Xero:  
  `./xerocli login`
* Fetch Bank Transactions for the default financial year:  
  `./xerocli bank-transactions`
* Fetch Invoices updated in the last 2 hours:  
  `./xerocli invoices --ago 2h`
* Fetch Bank Transactions for a specific financial year:  
  `./xerocli bank-transactions --fromDate 2024-04-01`
* Update the reference for a specific Bank Transaction:  
  `./xerocli bank-transaction-reference --uuid "..." --ref "NewReference123"`
* Wipe all local data and credentials:  
  `./xerocli wipe`

For more information on any command, use `--help`.
