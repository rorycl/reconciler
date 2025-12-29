package xero

import (
	"encoding/json"
	"os"
	"testing"
)

func TestAccountsType(t *testing.T) {
	b, err := os.ReadFile("testdata/accounts.json")
	if err != nil {
		t.Fatal(err)
	}
	var ar AccountResponse
	if err := json.Unmarshal(b, &ar); err != nil {
		t.Fatal(err)
	}
	if got, want := len(ar.Accounts), 90; got != want {
		t.Errorf("got %d accounts, want %d", got, want)
	}
}

func TestBankTransactionsType(t *testing.T) {
	b, err := os.ReadFile("testdata/bank_transactions.json")
	if err != nil {
		t.Fatal(err)
	}
	var bt BankTransactionsResponse
	if err := json.Unmarshal(b, &bt); err != nil {
		t.Fatal(err)
	}
	if got, want := len(bt.BankTransactions), 29; got != want {
		t.Errorf("got %d bank transactions, want %d", got, want)
	}
}

func TestInvoicesType(t *testing.T) {
	b, err := os.ReadFile("testdata/invoices.json")
	if err != nil {
		t.Fatal(err)
	}
	var i InvoiceResponse
	if err := json.Unmarshal(b, &i); err != nil {
		t.Fatal(err)
	}
	if got, want := len(i.Invoices), 88; got != want {
		t.Errorf("got %d invoices, want %d", got, want)
	}
}
