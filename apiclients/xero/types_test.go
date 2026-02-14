package xero

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestParseXeroDate(t *testing.T) {
	dateTimes := []string{
		`/Date(1301880520783+0000)/`,
		`/Date(1767010099219)/`,
		`\/Date(1770855011934)\/`,
	}
	for _, dt := range dateTimes {
		parsedDate, err := parseXeroDate(dt)
		if err != nil {
			t.Errorf("date string %q could not be parsed", dt)
		} else {
			fmt.Println(parsedDate)
		}
	}
}

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
	if got, want := string(bt.BankTransactions[0].BankAccountID), "bd9e85e0-0478-433d-ae9f-0b3c4f04bfe4"; got != want {
		t.Errorf("got %q bank account id, want %q for the first transaction", got, want)
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
