package web

import (
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/rorycl/reconciler/internal/token"
)

func TestRedirectCheck(t *testing.T) {

	gob.Register(time.Time{})
	gob.Register(token.ExtendedToken{})

	startDate := new(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	endDate := new(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))

	tests := []struct {
		name             string
		form             formURLer
		url              string
		sessionURL       string
		thisURL          string
		expectedURL      string
		expectedRedirect bool
		expectedErr      error
	}{
		{
			name:             "ok search form",
			form:             NewSearchForm(startDate, nil),
			url:              "/invoices?date-from=2025-06-01&date-to=2025-07-01&search=search_string&page=1",
			thisURL:          "/invoices",
			expectedURL:      "/invoices?date-from=2025-06-01&date-to=2025-07-01&page=1&search=search_string&status=NotReconciled",
			expectedRedirect: false,
			expectedErr:      nil,
		},
		{
			name:             "reset search form 1",
			form:             NewSearchForm(startDate, nil),
			url:              "/invoices?reset=true&date-from=2025-06-01&date-to=2025-05-01&search=search_string&page=1",
			thisURL:          "/invoices",
			expectedURL:      "/invoices?date-from=2026-02-01&date-to=2027-04-01&page=1&search=&status=NotReconciled",
			expectedRedirect: true,
			expectedErr:      nil,
		},
		{
			name:             "reset search form 2",
			form:             NewSearchForm(startDate, nil),
			url:              "/invoices?reset=true",
			thisURL:          "/invoices",
			expectedURL:      "/invoices?date-from=2026-02-01&date-to=2027-04-01&page=1&search=&status=NotReconciled",
			expectedRedirect: true,
			expectedErr:      nil,
		},
		{
			name:             "naked search form empty session",
			form:             NewSearchForm(startDate, nil),
			url:              "/invoices",
			thisURL:          "/invoices",
			expectedURL:      "/invoices?date-from=2026-02-01&date-to=2027-04-01&page=1&search=&status=NotReconciled",
			expectedRedirect: true,
			expectedErr:      nil,
		},
		{
			name:             "naked search form loaded session",
			form:             NewSearchForm(startDate, nil),
			url:              "/invoices",
			sessionURL:       "/invoices?date-from=2025-06-01&date-to=2025-07-01&page=1&search=search_string&status=NotReconciled",
			thisURL:          "/invoices",
			expectedURL:      "/invoices?date-from=2025-06-01&date-to=2025-07-01&page=1&search=search_string&status=NotReconciled",
			expectedRedirect: true,
			expectedErr:      nil,
		},
		{
			name:        "invalid search form",
			form:        NewSearchForm(startDate, nil),
			url:         "/invoices?x&n42=true&&",
			sessionURL:  "/invoices?date-from=2025-31-31&date-to=2025-07-01&page=1&search=search_string&status=NotReconciled",
			thisURL:     "/invoices",
			expectedErr: errors.New("url parameter decoding error"),
		},
		{
			name:             "donations form ok",
			form:             NewSearchDonationsForm(startDate, endDate),
			url:              `/donations?date-from=2025-06-01&date-to=2025-07-01&page=1&payout-reference=payout-ref&search=search+string&status=All`,
			thisURL:          "/donations",
			expectedURL:      "/donations?date-from=2025-06-01&date-to=2025-07-01&page=1&payout-reference=payout-ref&search=search+string&status=All",
			expectedRedirect: false,
			expectedErr:      nil,
		},
		{
			name:             "donations reset ok",
			form:             NewSearchDonationsForm(startDate, endDate),
			url:              `/donations?reset=true&date-from=2025-06-01&date-to=2025-07-01&page=1&payout-reference=payout-ref&search=search+string&status=All`,
			thisURL:          "/donations",
			expectedURL:      "/donations?date-from=2026-02-01&date-to=2026-03-01&page=1&payout-reference=&search=&status=NotLinked",
			expectedRedirect: true,
			expectedErr:      nil,
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {
			ctx := t.Context()
			sessions := scs.New()
			ctx, err := sessions.Load(ctx, "")
			if tt.sessionURL != "" {
				sessions.Put(ctx, tt.url, tt.sessionURL)
			}
			if err != nil {
				t.Fatalf("could not load session store: %v", err)
			}
			request := httptest.NewRequestWithContext(ctx, http.MethodGet, tt.url, nil)

			url, redirect, err := redirectCheck(ctx, tt.form, sessions, request, tt.thisURL)

			if err != nil && tt.expectedErr == nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err == nil && tt.expectedErr != nil {
				t.Fatalf("expected error: %v", tt.expectedErr)
			}
			if err != nil && tt.expectedErr != nil {
				if got, want := err.Error(), tt.expectedErr.Error(); !strings.Contains(got, want) {
					t.Errorf("error %s does not contain %s", got, want)
				}
			}
			if got, want := url, tt.expectedURL; got != want {
				t.Errorf("url got %q want %q", got, want)
			}
			if got, want := redirect, tt.expectedRedirect; got != want {
				t.Errorf("redirect got %t want %t", got, want)
			}
		})
	}
}
