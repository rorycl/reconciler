package web

import (
	"context"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

// formURLer is an interface for extracting the url from a URL-centric web form and
// decoding a url into the form, suitable for forms with a known good default state.
type formURLer interface {
	AsURLParams() (string, error)
	DecodeURLParams(map[string][]string) error
}

// redirectCheck determines if a request coming into an endpoint should be redirected if
// it is a search "reset" or "naked" url or otherwise what the current valid url should
// be. URLs which pass form parsing should not trigger a redirect. The func supports
// different forms which support the formURLer interface. The func returns the
// appropriate url, if any, if a redirect is needed and any error.
func redirectCheck(
	ctx context.Context,
	form formURLer,
	sessions *scs.SessionManager,
	r *http.Request,
	thisURL string) (string, bool, error) {

	// Determine default URL.
	urlParams, err := form.AsURLParams()
	if err != nil {
		return "", false, err
	}
	defaultURL := thisURL + "?" + urlParams

	// Reset case.
	if r.URL.Query().Get("reset") == "true" {
		_ = sessions.PopString(ctx, thisURL)
		return defaultURL, true, nil
	}

	// For a naked url, redirect to the saved or redirect url.
	if r.URL.RawQuery == "" {
		if savedURL := sessions.GetString(ctx, thisURL); savedURL != "" {
			return savedURL, true, nil
		}
		return defaultURL, true, nil
	}

	// Determine URL based on form values.
	if err := form.DecodeURLParams(r.URL.Query()); err != nil {
		return "", false, err
	}
	urlParams, err = form.AsURLParams()
	if err != nil {
		return "", false, err
	}
	currentURL := thisURL + "?" + urlParams

	return currentURL, false, nil
}
