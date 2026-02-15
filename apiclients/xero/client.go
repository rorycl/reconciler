package xero

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const baseURL = "https://api.xero.com/api.xro/2.0"

// APIClient is a wrapper for making authenticated calls to the Xero API.
type APIClient struct {
	httpClient *http.Client
	tenantID   string
	baseURL    string
}

// NewAPIClient creates a new Xero API client. If not httpClient is
// provided http.DefaultClient is used.
func NewAPIClient(tenantID string, httpClient *http.Client) *APIClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &APIClient{
		httpClient: httpClient,
		tenantID:   tenantID,
		baseURL:    baseURL,
	}
}

// GetBankTransactions fetches bank transactions from Xero, applying appropriate filters.
func (c *APIClient) GetBankTransactions(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]BankTransaction, error) {
	var allTransactions []BankTransaction
	page := 1

	for {
		// Build the 'where' clause for the query.
		var conditions []string
		conditions = append(conditions, `Type=="RECEIVE"`, `(Status=="AUTHORISED" OR Status=="PAID")`)
		toDate := fromDate.AddDate(1, 0, 0) // One year from the start date
		conditions = append(conditions, fmt.Sprintf(`Date >= DateTime(%d, %d, %d)`, fromDate.Year(), fromDate.Month(), fromDate.Day()))
		conditions = append(conditions, fmt.Sprintf(`Date < DateTime(%d, %d, %d)`, toDate.Year(), toDate.Month(), toDate.Day()))
		whereClause := strings.Join(conditions, " AND ")

		// Prepare the request URL with query parameters.
		params := url.Values{}
		params.Add("where", whereClause)
		params.Add("page", fmt.Sprintf("%d", page))
		requestURL := fmt.Sprintf("%s/BankTransactions?%s", c.baseURL, params.Encode())

		req, err := c.newRequest(ctx, "GET", requestURL, ifModifiedSince, nil)
		if err != nil {
			return nil, err
		}

		var response BankTransactionsResponse
		resp, err := do(c, req, &response)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request for page %d: %w", page, err)
		}

		// A 304 Not Modified response means no new data since the `If-Modified-Since` time.
		if resp.StatusCode == http.StatusNotModified {
			break
		}

		if len(response.BankTransactions) == 0 {
			break
		}

		allTransactions = append(allTransactions, response.BankTransactions...)
		page++
	}

	return allTransactions, nil
}

// GetInvoices fetches invoices from Xero, applying appropriate filters.
func (c *APIClient) GetInvoices(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]Invoice, error) {
	var allInvoices []Invoice
	page := 1

	for {
		var conditions []string
		conditions = append(conditions, `Type=="ACCREC"`, `(Status=="AUTHORISED" OR Status=="PAID" OR Status=="VOIDED")`)
		toDate := fromDate.AddDate(1, 0, 0)
		conditions = append(conditions, fmt.Sprintf(`Date >= DateTime(%d, %d, %d)`, fromDate.Year(), fromDate.Month(), fromDate.Day()))
		conditions = append(conditions, fmt.Sprintf(`Date < DateTime(%d, %d, %d)`, toDate.Year(), toDate.Month(), toDate.Day()))
		whereClause := strings.Join(conditions, " AND ")

		params := url.Values{}
		params.Add("where", whereClause)
		params.Add("page", fmt.Sprintf("%d", page))
		requestURL := fmt.Sprintf("%s/Invoices?%s", c.baseURL, params.Encode())

		req, err := c.newRequest(ctx, "GET", requestURL, ifModifiedSince, nil)
		if err != nil {
			return nil, err
		}

		var response InvoiceResponse
		resp, err := do(c, req, &response)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request for page %d: %w", page, err)
		}

		if resp.StatusCode == http.StatusNotModified {
			break
		}
		if len(response.Invoices) == 0 {
			break
		}

		allInvoices = append(allInvoices, response.Invoices...)
		page++
	}

	return allInvoices, nil
}

// GetAccounts fetches accounts from Xero, applying appropriate filters.
// There is no pagination.
func (c *APIClient) GetAccounts(ctx context.Context, ifModifiedSince time.Time) ([]Account, error) {

	requestURL := fmt.Sprintf("%s/Accounts", c.baseURL)

	req, err := c.newRequest(ctx, "GET", requestURL, ifModifiedSince, nil)
	if err != nil {
		return nil, err
	}

	var response AccountResponse
	resp, err := do(c, req, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request for accounts: %w", err)
	}

	if resp.StatusCode == http.StatusNotModified {
		return nil, nil
	}

	return response.Accounts, nil
}

// GetBankTransactionByID fetches a single bank transaction by its UUID.
func (c *APIClient) GetBankTransactionByID(ctx context.Context, uuid string) (BankTransaction, error) {
	requestURL := fmt.Sprintf("%s/BankTransactions/%s", c.baseURL, uuid)
	req, err := c.newRequest(ctx, "GET", requestURL, time.Time{}, nil)
	if err != nil {
		return BankTransaction{}, err
	}

	var response BankTransactionsResponse
	if _, err := do(c, req, &response); err != nil {
		return BankTransaction{}, err
	}

	if len(response.BankTransactions) == 0 {
		return BankTransaction{}, fmt.Errorf("bank transaction with UUID %s not found", uuid)
	}
	return response.BankTransactions[0], nil
}

// UpdateBankTransactionReference performs a POST request to update a transaction's reference.
// It returns the full, updated transaction object from the Xero API response.
func (c *APIClient) UpdateBankTransactionReference(ctx context.Context, tx BankTransaction, reference string) (BankTransaction, error) {
	tx.Reference = reference
	payload := map[string][]BankTransaction{"BankTransactions": {tx}}
	body, err := json.Marshal(payload)
	if err != nil {
		return BankTransaction{}, fmt.Errorf("failed to marshal update payload: %w", err)
	}

	requestURL := fmt.Sprintf("%s/BankTransactions", c.baseURL)
	req, err := c.newRequest(ctx, "POST", requestURL, time.Time{}, body)
	if err != nil {
		return BankTransaction{}, err
	}

	var response BankTransactionsResponse
	if _, err := do(c, req, &response); err != nil {
		return BankTransaction{}, err
	}

	if len(response.BankTransactions) == 0 {
		return BankTransaction{}, fmt.Errorf("update response did not contain a bank transaction")
	}
	return response.BankTransactions[0], nil
}

// newRequest is a helper to create a new HTTP request with common headers.
func (c *APIClient) newRequest(ctx context.Context, method, url string, ifModifiedSince time.Time, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("xero-tenant-id", c.tenantID)
	req.Header.Set("Accept", "application/json")
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	if !ifModifiedSince.IsZero() {
		req.Header.Set("If-Modified-Since", ifModifiedSince.UTC().Format(http.TimeFormat))
	}

	return req, nil
}

// do is a helper to execute an HTTP request and decode the JSON
// response. A nil `v` is supported for API calls not providing a
// response, such as DELETE calls.
func do[T any](c *APIClient, req *http.Request, v *T) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return resp, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Don't treat 304 Not Modified as an error, it's an expected response
		if resp.StatusCode == http.StatusNotModified {
			return resp, nil
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	_ = os.WriteFile("/tmp/xero.json", body, 0644)
	bodyReader := bytes.NewReader(body)

	if v != nil { // v might be nil for a DELETE request, for example.
		// if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		if err := json.NewDecoder(bodyReader).Decode(v); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return resp, nil
}

// GEThttps://api.xero.com/api.xro/2.0/Organisation

// getTenantID fetches the list of connections and returns the first TenantID found.
// Todo: check suitability of choosing the first connection.
func getTenantID(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", connectionsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create connections request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get connections: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error getting connections (status %d)", resp.StatusCode)
	}

	var connections []Connection
	if err := json.NewDecoder(resp.Body).Decode(&connections); err != nil {
		return "", fmt.Errorf("failed to decode connections response: %w", err)
	}

	if len(connections) == 0 {
		return "", fmt.Errorf("no tenants found for this connection")
	}

	return connections[0].TenantID, nil
}
