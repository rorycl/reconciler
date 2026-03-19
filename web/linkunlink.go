package web

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rorycl/reconciler/domain"
	"github.com/rorycl/reconciler/internal/token"
)

// handleDonationsLinkUnlink links or unlinks donations to either Xero invoices or bank
// transactions.
//
// The target here is hx-post="/donations/{{ .Typer }}/{{ .ID }}/(link|unlink)"
// However .ID is the bank-transaction or invoice UUID and the linking DFK info is
// the bank-transaction *reference* or invoice *invoice-number*. The DFK (and record
// date) is therefore retrieved using the `getInvoiceOrBankTransactionDetails` method.
func (web *WebApp) handleDonationsLinkUnlink() appHandler {

	return (func(w http.ResponseWriter, r *http.Request) error {

		ctx := r.Context()
		if r.Method != "POST" {
			return errHTMX{"only POST requests allowed", errors.New("non-POST request")}
		}

		// Extract url parameters.
		vars, err := validMuxVars(mux.Vars(r), "type", "id", "action")
		if err != nil {
			return errHTMX{"link/unlink: invalid mux vars", err}
		}

		// Extract the form data.
		err = r.ParseForm()
		if err != nil {
			return errHTMX{"form error", err}
		}

		// Validate the form data
		form, err := CheckLinkOrUnlinkForm(r.PostForm, vars)
		if err != nil {
			return errHTMX{"invalid form data", err}
		}
		validator := NewValidator()
		form.Validate(validator)
		if !validator.Valid() {
			return errHTMX{fmt.Sprintf("invalid data was received: %v", validator.Errors), errors.New("validator error")}
		}

		web.log.Debug(fmt.Sprintf("donationLinkUnlink %s action called for %s : %s (%d donations)",
			form.Action,
			form.Typer,
			form.ID,
			len(form.DonationIDs),
		))

		// In link mode, retrieve the details of the invoice or bank transaction.
		// retrieve the related invoice or bank transaction dfk and date
		var dfk string
		if form.Action == "link" {
			dfk, _, err = web.reconciler.InvoiceOrBankTransactionInfoGet(ctx, form.Typer, form.ID)
			if err != nil {
				if e, ok := errors.AsType[domain.ErrUsage](err); ok {
					return errHTMX{
						msg: e.Msg,
						err: e,
					}
				}
				return errInternal{
					msg: fmt.Sprintf("%T error: unexpected InvoiceOrBankTransactionInfoGet error", err),
					err: fmt.Errorf("link/unlink InvoiceOrBankTransactionInfoGet error: %w", err),
				}
			}
			if dfk == "" || dfk == missingTransactionReference {
				return errHTMX{
					fmt.Sprintf("%s id %s had cannot be linked", form.Typer, form.ID),
					errors.New("empty or invalid dfk"),
				}
			}
		}

		// Retrieve the oauth2 tokens from the session
		sfToken, err := web.getValidTokenFromSession(ctx, token.SalesforceToken)
		if err != nil {
			web.log.Info("sfToken empty, redirecting to connect")
			w.Header().Set("HX-Redirect", "/connect")
			w.WriteHeader(http.StatusOK)
			return nil
		}

		// Create the salesforce client.
		sfClient, err := web.newSFClient(ctx, web.cfg, web.log, sfToken)
		if err != nil {
			return errInternal{"failed to create salesforce client for linking/unlinking", err}
		}

		sfLastRefresh := web.sessions.GetTime(ctx, "sf-refreshed-datetime")

		// Run the Link/Unlink batch opportunity update and then upsert the results.
		err = web.reconciler.DonationsLinkUnlink(
			ctx,
			sfClient,
			form.AsSalesforceIDRefs(dfk),
			web.cfg.DataStartDate,
			sfLastRefresh.Add(refreshDurationWindow),
		)
		if err != nil {
			return err
		}
		web.log.Info("Successful donation opertions", "action", form.Action, "records", len(form.DonationIDs))

		// Redirect to the originator.
		// Todo: set focus to either the "find" or "linked" donations tab.
		redirectURL := fmt.Sprintf("/%s/%s/%s", form.Typer, form.ID, form.Action)
		w.Header().Set("HX-Redirect", redirectURL)

		w.WriteHeader(http.StatusOK)
		return nil

	})
}
