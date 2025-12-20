package salesforce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client is a wrapper for making authenticated calls to the Salesforce API.
type Client struct {
	httpClient    *http.Client
	instanceURL   string
	apiVersion    string
	inDevelopment bool
	dumpFile      string // path to dump http body response if inDevelopment
}

// GetOpportunities fetches Opportunity records from Salesforce, applying appropriate filters.
func (c *Client) GetOpportunities(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]Opportunity, error) {
	var conditions []string
	toDate := fromDate.AddDate(1, 0, 0) // One year from the start date
	conditions = append(conditions, fmt.Sprintf("CloseDate >= %s", fromDate.Format("2006-01-02")))
	conditions = append(conditions, fmt.Sprintf("CloseDate < %s", toDate.Format("2006-01-02")))

	if !ifModifiedSince.IsZero() {
		conditions = append(conditions, fmt.Sprintf("LastModifiedDate > %s", ifModifiedSince.UTC().Format(time.RFC3339)))
	}

	whereClause := strings.Join(conditions, " AND ")
	soql := fmt.Sprintf("SELECT Id, Name, Amount, CloseDate, StageName, RecordType.Name, LastModifiedDate, Payout_Reference__c FROM Opportunity WHERE %s", whereClause)

	requestURL := fmt.Sprintf("%s/services/data/%s/query?q=%s", c.instanceURL, c.apiVersion, url.QueryEscape(soql))
	req, err := c.newRequest(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, err
	}

	var response SOQLResponse
	if _, err := c.do(req, &response); err != nil {
		return nil, err
	}

	// The query API supports pagination, but for simplicity in this CLI,
	// we assume the result set is small enough to fit in one response.
	// A more robust implementation would check response.Done and query response.NextRecordsURL.
	return response.Records, nil
}

// BatchUpdateOpportunityRefs performs a batch update using the Salesforce Composite API.
func (c *Client) BatchUpdateOpportunityRefs(ctx context.Context, reference string, ids []string) error {
	if len(ids) > 25 {
		// The batch API is limited to 25 subrequests.
		// A more robust implementation would chunk the IDs into groups of 25.
		return fmt.Errorf("cannot update more than 25 records in a single batch")
	}

	batchRequest := BatchRequest{
		AllOrNone: false, // Continue processing even if one subrequest fails
		Requests:  make([]Subrequest, len(ids)),
	}

	for i, id := range ids {
		batchRequest.Requests[i] = Subrequest{
			Method: "PATCH",
			URL:    fmt.Sprintf("/services/data/%s/sobjects/Opportunity/%s", c.apiVersion, id),
			// The custom field API name must be correct, e.g., "Payout_Reference__c"
			Body: map[string]string{"Payout_Reference__c": reference},
		}
	}

	body, err := json.Marshal(batchRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal batch request: %w", err)
	}

	requestURL := fmt.Sprintf("%s/services/data/%s/composite/batch", c.instanceURL, c.apiVersion)
	req, err := c.newRequest(ctx, "POST", requestURL, body)
	if err != nil {
		return err
	}

	var response BatchResponse
	if _, err := c.do(req, &response); err != nil {
		return err
	}

	// Check for errors within the batch response results.
	for _, result := range response.Results {
		if result.StatusCode < 200 || result.StatusCode >= 300 {
			// A simple error handling approach; a robust one would collect all errors.
			errorBody, _ := json.Marshal(result.Result)
			return fmt.Errorf("batch subrequest failed with status %d: %s", result.StatusCode, string(errorBody))
		}
	}

	return nil
}

// newRequest is a helper to create a new HTTP request with common headers.
func (c *Client) newRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// do is a helper to execute an HTTP request and decode the JSON response.
func (c *Client) do(req *http.Request, v interface{}) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	if c.inDevelopment && c.dumpFile != "" {
		_ = os.WriteFile(c.dumpFile, body, 0644)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	if v != nil {
		// if err := json.UnmarshalNewDecoder(resp.Body).Decode(v); err != nil {
		if err := json.Unmarshal(body, v); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return resp, nil
}
