## Reconciler Security

**Revision E: 5 March 2026**

### Introduction

The `Reconciler App` is an open-source (MIT-licensed) desktop web app
for helping charities reconcile their financial data between their
financial and CRMS systems, currently focusing on Salesforce and Xero.

Reconciler acts as an Application-Programming Interface (API) client to
donation-based data in the remote platforms to read Xero and Salesforce
data, and to update one field in Salesforce Opportunity records to
reconcile records.

Reconciler requires a Salesforce Admisterator and Xero user with
'Standard' or 'Adviser' rights to generate API keys to configure OAuth2
PKCE access on their platform instances. These credentials are used in
the App to guide users through OAuth2 logins with their own credentials
to link Reconciler to the platforms. Users without access to both the
organisation's Salesforce and Xero instances will not be able to use
Reconciler.

The App database and session tokens run entirely in memory for a maximum
period of 8 hours. OAuth2 token lifetime is shorter and will require the
user to login again after periods of inactivity.

More information is provided at the project [Github
repository](https://github.com/rorycl/reconciler).

### Key guidance

* Users of the Reconciler App should be cognisant of their personal and
  organisational responsibilities relating to working with the
  Personally Identifiable Information (PII) data held in the system.

* Platform OAuth2 connections must be configured with PKCE verification,
  minimised permissions and the requirement for users to log in with
  their own credentials. User credentials with Multi-Factor
  Authentication (MFA) are recommended.

* The App's configuration file should be kept on a filesystem with
  robust protection against unauthorized access.

* The App should be used on a machine protected by a firewall.

* The App operates only on the `127.0.0.1` or `localhost` address for
  security reasons and cannot be shared over the network.

* Reconciler communicates with only the Xero and Salesforce platforms
  and no other platforms or services.

* Closing the App will require restarting and reconnecting to the
  platforms.

### Mode of operation

Reconciler is provided as a compiled Go binary from the project
repository. The App is a cross-platform local webapp, to be run
on a local desktop, and configured with a local configuration
`yaml` file. An example configuration file is provided
[here](https://github.com/rorycl/reconciler/blob/main/config/config.example.yaml).
SHA 256 checksums are provided for binary releases.

The App requires OAuth2 connection permissions to be created on the
remote platforms. (Separate guidance for administrators to configure
[Salesforce](https://github.com/rorycl/reconciler/blob/main/docs/salesforce_api_access_revc.pdf)
and [Xero](https://github.com/rorycl/reconciler/blob/main/docs/xero_api_access_reva.pdf)
API access are provided separately.) Both platforms require PKCE code
verification.

Users of the App should authenticate with their own credentials via the
browser. Only users who are able to successfully login to both remote
platforms will be able to use the App.

Platform access OAuth2 permission tokens and database are held in-memory
by the App. These are deleted on app closure or after 8 hours. After a
two hour period of inactivity the connection tokens time out, requiring
the user to log in again.

For Xero, only read-only API connections are made. The chart of account
and some organisation details are retrieved. All donation-related
invoices and bank transactions (those line items with account codes
matching the configured `donation_account_prefixes`) are also retrieved.

For Salesforce, sparse information is retrieved from the Opportunities
(also known as "Donations") object as set out in the configured
`salesforce.query` SOQL query.

The App works to add Xero codes to a target field in Salesforce donation
records. This is the only data changing operation made by the App.

### Security considerations

Broadly the security considerations fall into three areas.

1. Authentication, authorization and connection security
2. Filesystem security
3. Platform data

#### Authentication, Authorization and Connection security

For each API connection OAuth2 PKCE credentials are needed, together
with authentication through the user's personal login credentials.
Consequently, all Reconciler users will need to have been setup
individually as authorized users to each remote platform. These profiles
*are recommended to be* MFA (multi-factor authentication) protected.

The OAuth2 token issuance and refresh flow is an industry standard
for remote platforms issuing short-lived access tokens for access
to platform resources. These tokens are held in App memory and are
deleted on App closure or after a 2 hour inactivity timeout. The OAuth2
process is further strengthened by using PKCE code verification.

CSRF attacks are mitigated by [CSRF
protection](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
built into the App.

#### Filesystem security

Reconciler is configured with a local yaml-format file on disk. The
configuration file with its OAuth2 application credentials should be in
a suitably protected filesystem location.

Access to the configuration file is unlikely to allow an attacker to
cause damage, as each user is required to provide additional login
credentials to access the platforms. Nevertheless, any suspected leak of
credentials should be met with the immediate suspension of the platform
connection setups.

No database or OAuth2 tokens are stored on disk.

#### Platform Data

While the Reconciler App is designed to only update a single custom
field in Salesforce, it is possible a malicious user who has access
to the configuration file and platforms could cause another field
or fields in Salesforce to be updated. Since the user's credentials
will be recorded against any such change, this risk is similar to
that posed by a malicious local user of Salesforce.
