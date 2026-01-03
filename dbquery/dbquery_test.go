package dbquery

import (
	"context"
	"testing"
	"time"
)

func TestInvoicesQuery(t *testing.T) {

	ctx := context.Background()

	accountCodes := "^(53|55|57)"

	db, err := New("test.db", accountCodes)
	if err != nil {
		t.Fatalf("db opening error: %v", err)
	}

	var reconciled = true
	_, err = db.GetInvoices(ctx, &reconciled, time.Time{}, time.Time{}, "")
	if err != nil {
		t.Fatalf("get invoices error: %v", err)
	}

}
