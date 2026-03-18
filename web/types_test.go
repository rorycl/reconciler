package web

import (
	"context"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/domain"
	"github.com/rorycl/reconciler/internal/token"
	"golang.org/x/oauth2"
)

// TestTypeInterfaces tests newDefaultXeroClient and NewDefaultSalesforceClient
// interface converters.
func TestTypeInterfaceConverters(t *testing.T) {

	validToken := &token.ExtendedToken{
		Type:        token.SalesforceToken,
		InstanceURL: "https://instance-url-example",
		Token: &oauth2.Token{
			AccessToken: "valid-token-123",
			Expiry:      time.Now().Add(1 * time.Hour), // not expired
		},
	}

	sc, err := newDefaultSalesforceClient(context.Background(), &config.Config{}, slog.Default(), validToken)
	if err != nil {
		t.Fatal(err)
	}
	switch sc.(type) {
	case domain.SalesforceClient:
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

	xc, err := newDefaultXeroClient(context.Background(), slog.Default(), regexp.MustCompile("."), validToken)
	if err != nil {
		t.Fatal(err)
	}
	switch xc.(type) {
	case domain.XeroClient:
	default:
		t.Errorf("expected xeroClienter type, got %T", xc)
	}
}
