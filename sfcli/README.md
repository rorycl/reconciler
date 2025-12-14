# sfcli

A cli for interacting with salesforce.

- **Login to Salesforce:**  
  `./sfcli login`

- **Fetch Opportunities for the default date range:**  
  `./sfcli opportunities`

- **Fetch Opportunities updated in the last day:**  
  `./sfcli opps --ago 24h`

- **Fetch Opportunities for a specific date range:**  
  `./sfcli opps --fromDate 2024-04-01`

- **Update the Payout Reference for multiple Opportunities:**  
  `./sfcli opportunities-ref --ref "XERO-REF-123" --ids "0063..,0067..,0063.."`

- **Wipe all local data and credentials:**  
  `./sfcli wipe`

For more information on any command, use the `--help` flag.  
e.g. `./sfcli opportunities --help`
