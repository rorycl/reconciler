# Reconciler Security

**Revision C 21 February 2026**

The `Reconciler App` is a MIT-licensed desktop web app for helping
charities reconcile their financial data between their financial and
CRMS systems, currently focusing on Salesforce and Xero.

Reconciler App users are required to have individual logins
to Salesforce and Xero to use the App for reconciliation. The
Reconciler App draws records over OAuth2 secured API connections into
a local database to assist users to update a field in Salesforce
donation records to match an identifier from Xero invoices or bank
transactions. This process allows an organisation to bring the
donation totals for Xero income into reconciliation with the related
Salesforce donations.

This document focuses solely on the desktop App, sets out some of
the security considerations of using the App and is written for
a UK context. More information is provided at the project [Github
repository](https://github.com/rorycl/reconciler).

## Key guidance

* Users of the Reconciler App should be cognisant of their personal
and organisational responsibilities relating to working with the
Personally Identifiable Information (PII) data held in the system.

* Platform OAuth2 connections must be configured with PKCE verification,
  minimum permissions and the requirement for users to log in with their
  own credentials. User credentials with MFA are recommended.

* The App should be used on a machine protected by a firewall.

* The App will only operate on the `127.0.0.1` or `localhost` address
  for security reasons and must not be shared over the network.

* The App's configuration file should kept on a filesystem with robust
  protection against unauthorized access.

* The App should be configured either to use an in-memory database,
  which is automatically deleted when the app is closed, or to store its
  database on a filesystem with robust protection against unauthorized
  access. For security an in-memory database is recommended.

* Users should be aware that closing the App will require them to log in
  again. After two hours of inactivity the current OAuth2 tokens held in
  memory will be deleted, also requiring the user to log in again.

* Users utilising an on-file database should delete the database when it
  is no longer needed.

## Mode of operation

Reconciler is provided as a compiled Go binary from the project
repository. The App is a cross-platform local webapp, to be run
on a local desktop, and configured with a local configuration
`yaml` file. An example configuration file is provided
[here](https://github.com/rorycl/reconciler/blob/main/config/config.example.yaml).

The App requires OAuth2 connection permissions to be created on the
remote platforms. Separate guidance is available for configuring
these connections. It is important that platform access permissions
are minimised and that PKCE verification is enforced.

Users of the App should be required to authenticate with their own
credentials via the browser. Only users who are able to successfully
login to both remote platforms will be able to use the App. Access
permission tokens are only held in memory on the local machine and
closing the App or a two hour period of inactivity will delete the
tokens, requiring the user to log in again.

Connection to each remote platform is done through OAuth2 connection
flows using the credentials and scopes set out in the configuration
file. Successful connections result in a token file for each service
being saved in App memory. This token is automatically refreshed
using the OAuth2 refresh process when applicable.

Generally data is only retrieved from the configuration
`data_date_start` where applicable. The initial data retrieval and
insertion into the local database may take several minutes.

For Xero, only read-only API connections are made. All accounts
records are stored in the local database, as are all invoices and
bank transactions which have donation-related line items as defined
by the configuration `donation_account_prefixes`. In future Xero
`Organisation` info is also likely to be needed to construct Xero
`deep links`.

For Salesforce, sparse information is retrieved from the Opportunities
(also known as "Donations") object as configured by the configured
`salesforce.query` SOQL query.

After connection and data refresh, the App provides operations
for searching Bank Transactions, Invoices and Donations,
and then reconciling the former two record types with the
last. To achieve this, the local Bank Transaction `Reference` or
Invoice `Invoice Number` field is used to populate the configured
`inking_object.linking_field_name` record in Salesforce. This "linking"
(or "unlinking") action is the only data changing action performed by
the Reconciler App. The operations of the App are protected against
cross-site request forgery (CSRF) attacks.

Upon logging out the local json OAuth2 tokens are deleted from memory.
Any in-memory database is also deleted.

## Security considerations

Broadly the security considerations fall into three areas.

1. Authentication, authorization and connection security
2. Filesystem security
3. Platform data

### Authentication, Authorization and Connection security

Users of the Reconciler App should be cognisant of their personal
and organisational responsibilities relating to working with the
Personally Identifiable Information (PII) that may be held in the
system. Since users of the Reconciler App will also be users of Xero
and Salesforce the PII controls for these services should be equally
Applied to the use of the Reconciler App.

For each API connection a `client_id` and `client_secret` is needed,
together with authentication through the user's login credentials
which *are recommended to be* MFA (multi-factor authentication)
protected. The user also needs to have been authorized to access the
remote resources in question.

The OAuth2 token issuance and refresh flow is an industry standard
for remote platforms issuing short-lived access tokens for access
to the target resources. These tokens are held in App memory and are
deleted on App closure or after a 2 hour inactivity timeout. The OAuth2
process is further strengthened by using PKCE code verification. CSRF
attacks are mitigated by [CSRF protection](http://../web/csrf.go)
built into the local webApp, which only allows access to modern
browsers supporting the `Sec-Fetch-Site` and `Origin` headers. Legacy
browsers are not supported.

Connections to the remote platforms are defined as `https` encrypted
to protect the security of communications. The local machine using
the Reconciler App should be protected by a high-quality firewall.

### Filesystem security

As presently set up, Reconciler draws its configuration from a
configuration file on disk and stores the remote records either in a
local sqlite database on disk or in memory. For security, the latter is
recommended.

Access to the configuration file is unlikely to allow an attacker to
cause damage, as each user is required to provide additional login
credentials to access the platforms. Nevertheless, the configuration
file and credentials should be protected. Any suspected leak of
credentials should be met with the immediate suspension of the
connection setup on the platforms.

Access to any on disk database might allow an attacker to access the
records in the system, including the Personally Identifiable Information
(PII) therein relating to donations. While it is possible to run the
database in memory this is likely to be inconvenient due to memory
consumption and the time required to resynchronise all records. The
database should be stored on a filesystem with robust protection against
unauthorized access.

Users should delete any local App database when it is no longer
required. For security, the use of an in-memory database is recommended
which obviates this need by deleting the local database on closure.

### Platform Data

While the Reconciler App is designed to only update a single custom
field in Salesforce, it is possible a malicious user who has access
to the configuration file and platforms could cause another field
or fields in Salesforce to be updated. Since the user's credentials
will be recorded against any such change, this risk is similar to
that posed by any malicious local user of Salesforce.
