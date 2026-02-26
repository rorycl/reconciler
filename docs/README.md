## reconciler docs

The `reconciler` app is a cross-platform desktop webapp for charities to
reconcile their accounts and CRMS systems to search, align and audit
donation income and records, presently focusing on Xero and Salesforce.

The app uses OAuth2 PKCE code verified connections to Xero and
Salesforce. Configuration of the connections by administrative users
allows other users with the necessary permissions to use the reconciler
app after logging in with their own credentials.

The documentation here relates to the security of the system and setting
up the API connections.

### Index

* **Security**  
  The security considerations of using the app are set out in the
  [security document](https://github.com/rorycl/reconciler/blob/main/docs/security.md).

* **Salesforce API configuration guide**  
  A [guide](https://github.com/rorycl/reconciler/blob/main/docs/salesforce_api_access_revc.pdf)
  or setting up Salesforce API access.

* **Xero API configuration guide**  
  A [guide](https://github.com/rorycl/reconciler/blob/main/docs/xero_api_access_reva.pdf)
  or setting up Xero API access.

