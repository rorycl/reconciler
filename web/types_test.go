package web

import (
	"context"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/internal/token"
	"golang.org/x/oauth2"
)

// TestTypeInterfaces tests the newXeroClienter
// and newSalesforceClienter interface converters.
func TestTypeInterfaceConverters(t *testing.T) {

	validToken := &token.ExtendedToken{
		Type:        token.SalesforceToken,
		InstanceURL: "https://instance-url-example",
		Token: &oauth2.Token{
			AccessToken: "valid-token-123",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
	}

	sc, err := newSalesforceClienter(context.Background(), &config.Config{}, slog.Default(), validToken)
	if err != nil {
		t.Fatal(err)
	}
	switch sc.(type) {
	case sfClienter:
	default:
		t.Errorf("expected sfClienter type, got %T", sc)
	}

	validToken = &token.ExtendedToken{
		Type:        token.XeroToken,
		InstanceURL: "https://instance-url-example",
		Token: &oauth2.Token{
			AccessToken: "valid-token-123",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
		TenantID: "tenant-1",
	}

	xc, err := newXeroClienter(context.Background(), slog.Default(), regexp.MustCompile("."), validToken)
	if err != nil {
		t.Fatal(err)
	}
	switch xc.(type) {
	case xeroClienter:
	default:
		t.Errorf("expected xeroClienter type, got %T", xc)
	}
}
