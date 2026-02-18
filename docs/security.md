# Reconciler Security

**Revision A 18 February 2026**

The `Reconciler App` is a MIT-licensed desktop web app for helping
charities reconcile their financial data between their financial and
CRMS systems, currently focusing on Salesforce and Xero.

Reconciler draws records over OAuth2 secured API connections into a
local database to assist users to update a field in Salesforce donation
records to match an identifier from Xero invoices or bank transactions.
This process allows an organisation to bring the donation totals for
Xero income into reconciliation with the related Salesforce donations.

This document sets out some of the security considerations of using this
app. This document is written for a UK context.

## Key guidance

* Users of the Reconciler app should be cognisant of their personal and
  organisational responsibilities relating to working with the
  Personally Identifiable Information (PII) data held in the system. 

* Platform OAuth2 connections must be properly configured with PKCE
  verification, minimum permissions and the requirement for users to log
  in with their own credentials. User credentials with MFA are
  recommended.

* The Reconciler app should be used on a machine protected by a
  firewall.

* The app will only operate on the `127.0.0.1` or `localhost` address
  for security reasons and should not be shared over the network.

* The Reconciler app should be configured to store its OAuth2 platform
  access token files and sqlite database on a filesystem with robust
  protection against unauthorized access.

* Users should ensure they logout of the Reconciler app to remove their
  OAuth2 platform access tokens to protect against malicious use of the
  tokens. Closing the application window or stopping the server does
  **not** automatically remove this tokens.

* Users should delete the local Reconciler app database when it is no
  longer needed.

## Mode of operation

Reconciler is provided as a compiled Go binary from the project
repository at [github](https://github.com/rorycl/reconciler). The app is
a cross-platform local webapp, designed to be run on a local desktop,
and configured with a local configuration `yaml` file. An example
configuration file is provided [here](../config/config.example.yaml).

The app requires OAuth2 connection permissions to be created on the
remote platforms. Separate guidance is available for configuring these
connections. It is important that platform access permissions are
minimised and that PKCE verification is enforced.

Users of the app should be required to authenticate with their own
credentials via the browser. Only users who are able to successfully
login to both remote platforms will be able to use the app. However,
once authenticated, access persists on the local machine via stored
tokens until the user explicitly logs out or the tokens expire.

Connection to each remote platform is done through OAuth2 connection
flows using the credentials and scopes set out in the configuration
file. Successful connections result in a token file for each service
being saved to the relevant configuration file path. This token is
automatically refreshed using the OAuth2 refresh process when
applicable. The app ensures that tokens cannot be used more than 12
hours after last use.

After successful connection, the app connects to both platforms to
retrieve or refresh data, which is stored in a local database at the
configured `database_path`. Generally data is only retrieved from the
configuration `data_date_start` where applicable. The initial data
retrieval and insertion into the local database may take several minutes.

For Xero, only read-only API connections are made. All accounts
descriptions are stored in the local database, as are all invoices and
bank transactions which have donation-related line items as defined by
the configuration `donation_account_prefixes`. In future Xero
`Organisation` info is also likely to be needed to construct Xero `deep
links`.

For Salesforce, sparse information is retrieved from the Opportunities
(also known as "Donations") object as configured by the
`salesforce.query` SOQL query. 

After connection and data refresh, the app provides operations for
searching Bank Transactions, Invoices and Donations, and then
reconciling the former two record types with the last. To achieve this,
the local Bank Transaction `Reference` or Invoice `Invoice Number` field
is used to populate the configured `linking_object.linking_field_name`
record in Salesforce. This "linking" (or "unlinking") action is the only
data changing action performed by the Reconciler app. The operations of
the app are protected against cross-site request forgery (CSRF) attacks.

Upon logging out the local json OAuth2 tokens are deleted. The database
is retained to avoid having to resynchronise all records, in order to
speed up future operations.

## Security considerations

Broadly the security considerations fall into three areas.

1. Authentication, authorization and connection security
2. Filesystem security
3. Platform data 

### Authentication, Authorization and Connection security

Users of the Reconciler app should be cognisant of their personal and
organisational responsibilities relating to working with the Personally
Identifiable Information (PII) that may be held in the system. Since
users of the Reconciler app will also be users of Xero and Salesforce
the PII controls for these services should be equally applied to the use
of the Reconciler app. 

For each API connection a `client_id` and `client_secret` is needed,
together with authentication through the user's login credentials which
*are recommended to be* MFA (multi-factor authentication) protected. The
user also needs to have been authorized to access the remote resources
in question.

The OAuth2 token issuance and refresh flow is an industry standard for
remote platforms issuing short-lived access tokens for access to the
target resources. This process is further strengthened by using PKCE
code verification. CSRF attacks are mitigated by [CSRF
protection](../web/csrf.go) built into the local webapp, which only
allows access to modern browsers supporting the `Sec-Fetch-Site` and
`Origin` headers. Legacy browsers are not supported.

Connections to the remote platforms are defined as `https` encrypted to
protect the security of communications. The local machine using the
Reconciler app should be on a network protected by a high-quality
firewall.

### Filesystem security

As presently set up, Reconciler draws its configuration from a
configuration file on disk, saves the OAuth2 token files to disk, and
stores the remote records in a local sqlite database.

Access to the configuration file is unlikely to allow an attacker to
cause damage, as each user is required to provide additional login
credentials to access the platforms. Nevertheless, the configuration
file and credentials should be protected. Any suspected leak of
credentials should be met with the immediate suspension of the
connection setup on the platforms.

Access to the OAuth2 token files could allow an attacker to access the
remote resources with the permissions of the user during the period of
validity of the token (or its `refresh` component). It is **strongly**
recommended that users always use the Log Out facility (at `/logout`) to
remove these tokens after use. This will require them to re-connect to
each platform using their personal credentials at next use. The token
files should be stored on a filesystem with robust protection against
unauthorized access.

Failure to remove the tokens could allow other users to access the
remote system with the permissions of the originating user.

An attacker with currently valid OAuth2 tokens generated by another user
could use these to modify, create and delete data on Salesforce with the
permissions of the user using a custom applications or scripts.

Access to the database on disk would allow an attacker to access the
records in the system, including the Personally Identifiable Information
(PII) therein relating to donations. While it is possible to run the
database in memory this is likely to be inconvenient due to memory
consumption and the time required to resynchronise all records. The
database should be stored on a filesystem with robust protection against
unauthorized and malicious access.

Users should delete the local Reconciler app database when it is no
longer required.

### Platform Data

While the Reconciler app is designed to only update a single custom
field in Salesforce, it is possible a malicious user who has access to
the configuration file and platforms could cause another field or fields
in Salesforce to be updated. Since the user's credentials will be
recorded against any such change, this risk is similar to that posed by
a generally malicious user of Salesforce.
