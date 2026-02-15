// This test is largely taken from csrf_test.go from net/http, Copyright the Go Authors
// 2025, covered under a BSD-style license.

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// httptestNewRequest works around https://go.dev/issue/73151.
func httptestNewRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.URL.Scheme = ""
	req.URL.Host = ""
	return req
}

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// TestEnforceCSRF tests both enforceCSRF and preventCSRF.
// This is modified from net/http.csrf_test.go's TestCrossOriginProtectionSecFetchSite
func TestEnforceCSRF(t *testing.T) {

	handler := enforceCSRF(okHandler)
	// handler := preventCSRF(okHandler)

	tests := []struct {
		name           string
		method         string
		secFetchSite   string
		origin         string
		expectedStatus int
	}{
		/*
		   --- PASS: TestEnforceCSRF/same-origin_allowed (0.00s)
		   --- PASS: TestEnforceCSRF/none_not_allowed (0.00s)
		   --- FAIL: TestEnforceCSRF/cross-site_blocked (0.00s)
		   --- FAIL: TestEnforceCSRF/same-site_blocked (0.00s)
		   --- PASS: TestEnforceCSRF/GET_allowed (0.00s)
		   --- PASS: TestEnforceCSRF/no_header_with_no_origin (0.00s)
		   --- PASS: TestEnforceCSRF/no_header_with_matching_origin (0.00s)
		   --- FAIL: TestEnforceCSRF/no_header_with_mismatched_origin (0.00s)
		   --- FAIL: TestEnforceCSRF/no_header_with_null_origin (0.00s)
		   --- PASS: TestEnforceCSRF/GET_allowed#01 (0.00s)
		   --- PASS: TestEnforceCSRF/HEAD_allowed (0.00s)
		   --- PASS: TestEnforceCSRF/OPTIONS_allowed (0.00s)
		   --- FAIL: TestEnforceCSRF/PUT_blocked (0.00s)
		*/
		// All should fail except "same-origin"
		{"same-origin allowed", "POST", "same-origin", "", http.StatusOK},
		{"none not allowed", "POST", "", "", http.StatusForbidden},
		{"cross-site blocked", "POST", "cross-site", "", http.StatusForbidden}, // fail
		{"same-site blocked", "POST", "same-site", "", http.StatusForbidden},   // fail

		{"GET allowed", "GET", "", "", http.StatusOK},
		{"POST without headers blocked", "POST", "", "", http.StatusForbidden},

		// All should be ok other than those coming from origin https://example.com.
		{"no header with no origin", "POST", "", "", http.StatusForbidden},
		{"no header with matching origin", "POST", "", "https://example.com", http.StatusOK},
		{"no header with mismatched origin", "POST", "", "https://attacker.example", http.StatusForbidden}, // fail
		{"no header with null origin", "POST", "", "null", http.StatusForbidden},                           // fail

		// All should be ok except for PUT without same-origin.
		{"GET allowed", "GET", "cross-site", "", http.StatusOK},
		{"HEAD allowed", "HEAD", "cross-site", "", http.StatusOK},
		{"OPTIONS allowed", "OPTIONS", "cross-site", "", http.StatusOK},
		{"PUT blocked", "PUT", "cross-site", "", http.StatusForbidden}, // fail
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptestNewRequest(tc.method, "https://example.com/")
			if tc.secFetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.secFetchSite)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("got status %d, want %d", w.Code, tc.expectedStatus)
			}
		})
	}
}
