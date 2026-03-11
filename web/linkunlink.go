package web

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rorycl/reconciler/internal/token"
)

// handleDonationsLinkUnlink links or unlinks donations to either Xero invoices or bank
// transactions.
//
// The target here is hx-post="/donations/{{ .Typer }}/{{ .ID }}/(link|unlink)"
// However .ID is the bank-transaction or invoice UUID and the linking DFK info is
// the bank-transaction *reference* or invoice *invoice-number*. The DFK (and record
// date) is therefore retrieved using the `getInvoiceOrBankTransactionDetails` method.
func (web *WebApp) handleDonationsLinkUnlink() http.Handler {

	dataStartDate := web.cfg.DataStartDate

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		ctx := r.Context()
		if r.Method != "POST" {
			web.htmxClientError(w, "only POST requests allowed")
			return
		}

		// Extract url parameters.
		vars, err := validMuxVars(mux.Vars(r), "type", "id", "action")
		if err != nil {
			web.log.Error(fmt.Sprintf("link/unlink error: invalid mux vars: %v", err))
			web.htmxClientError(w, err.Error())
			return
		}

		// Extract the form data.
		err = r.ParseForm()
		if err != nil {
			web.log.Error(fmt.Sprintf("%s form error: invalid POST request: %v", vars["action"], err))
			web.htmxClientError(
				w,
				fmt.Sprintf("%s form error: invalid POST request", vars["action"]),
			)
			return
		}

		// Validate the form data
		form, err := CheckLinkOrUnlinkForm(r.PostForm, vars)
		if err != nil {
			web.log.Error(fmt.Sprintf("%s form error: invalid form data: %v", vars["action"], err))
			web.htmxClientError(
				w,
				fmt.Sprintf("%s form error: invalid form data", vars["action"]),
			)
			return
		}
		validator := NewValidator()
		form.Validate(validator)
		if !validator.Valid() {
			web.log.Error(fmt.Sprintf("invalid data was received: %v", validator.Errors))
			web.htmxClientError(
				w,
				fmt.Sprintf("%s form error: invalid data was received", vars["action"]))
			return
		}

		web.log.Info(fmt.Sprintf("donationLinkUnlink %s action called for %s : %s (%d donations)",
			form.Action,
			form.Typer,
			form.ID,
			len(form.DonationIDs),
		))

		// In link mode, retrieve the details of the invoice or bank transaction.
		// retrieve the related invoice or bank transaction dfk and date
		var dfk string
		if form.Action == "link" {
			dfk, _, err = web.getInvoiceOrBankTransactionDetails(ctx, form.Typer, form.ID)
			if err != nil {
				web.log.Error(fmt.Sprintf("could not get invoice or bank transaction info: %v", err))
				web.htmxClientError(
					w,
					fmt.Sprintf("%s id: %s error: could not get invoice/transaction info", form.Typer, form.ID))
				return
			}
			if dfk == "" || dfk == missingTransactionReference {
				web.log.Error(fmt.Sprintf("%s id %s had empty or invalid dfk and cannot be linked", form.Typer, form.ID))
				web.htmxClientError(
					w,
					fmt.Sprintf("%s id %s has an empty dfk and cannot be linked", form.Typer, form.ID))
				return
			}
		}

		// Retrieve the oauth2 tokens from the session
		sfToken, err := web.getValidTokenFromSession(ctx, token.SalesforceToken)
		if err != nil {
			web.log.Info("sfToken empty, redirecting to connect")
			w.Header().Set("HX-Redirect", "/connect")
			w.WriteHeader(http.StatusOK)
			return
		}

		// Create the salesforce client.
		sfClient, err := web.newSFClient(ctx, web.cfg, web.log, sfToken)
		if err != nil {
			web.ServerError(w, r, fmt.Errorf("failed to create salesforce client for linking/unlinking: %w", err))
			return
		}

		sfLastRefresh := web.sessions.GetTime(ctx, "sf-refreshed-datetime")

		// Update the donations. If it is an unlink action, update the dfk with "", else
		// the actual dfk from the bank transaction or invoice. The form contents (many
		// salesforce IDs given the same DFK reference) must be translated to
		// a slice of salesforce.IDRef, hence the use of form.AsSalesforceIDRefs.
		_, err = sfClient.BatchUpdateOpportunityRefs(ctx, form.AsSalesforceIDRefs(dfk), false)
		if err != nil {
			web.ServerError(w, r, fmt.Errorf("failed to batch update salesforce records for linking/unlinking: %w", err))
			return
		}
		web.log.Info(fmt.Sprintf("Successfully linked %d donations.", len(form.DonationIDs)))

		// Upsert the updated opportunities.
		// The refresh window is rough; double upserts shouldn't be a major issue.
		updatedDonations, err := sfClient.GetOpportunities(ctx, dataStartDate, sfLastRefresh.Add(refreshDurationWindow))
		if err != nil {
			web.ServerError(w, r, fmt.Errorf("failed to upsert the linked opportunities: %v", err))
			return
		}
		if err := web.db.UpsertDonations(ctx, updatedDonations); err != nil {
			web.ServerError(w, r, fmt.Errorf("failed to save updated donations to local DB: %v", err))
			return
		}

		// Redirect to the originator.
		// Todo: set focus to either the "find" or "linked" donations tab.
		redirectURL := fmt.Sprintf("/%s/%s/%s", form.Typer, form.ID, form.Action)
		w.Header().Set("HX-Redirect", redirectURL)
		w.WriteHeader(http.StatusOK)

	})
}
