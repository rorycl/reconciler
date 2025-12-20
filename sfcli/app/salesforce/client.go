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

	"sfcli/app" // Import the app package to access config structs
)

// Client is a wrapper for making authenticated calls to the Salesforce API.
type Client struct {
	httpClient  *http.Client
	instanceURL string
	apiVersion  string
	config      app.SalesforceConfig
}

// GetOpportunities fetches records from Salesforce using the configurable SOQL query.
func (c *Client) GetOpportunities(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]Record, error) {
	var conditions []string
	toDate := fromDate.AddDate(1, 0, 0) // One year from the start date
	conditions = append(conditions, fmt.Sprintf("CloseDate >= %s", fromDate.Format("2006-01-02")))
	conditions = append(conditions, fmt.Sprintf("CloseDate < %s", toDate.Format("2006-01-02")))

	if !ifModifiedSince.IsZero() {
		conditions = append(conditions, fmt.Sprintf("LastModifiedDate > %s", ifModifiedSince.UTC().Format(time.RFC3339)))
	}
	whereClause := strings.Join(conditions, " AND ")

	// Replace the placeholder in the query template with the generated WHERE clause.
	finalSOQL := strings.Replace(c.config.Query, "{{.WhereClause}}", whereClause, 1)

	// Dump the final query for debugging purposes.
	_ = os.WriteFile("salesforce_query.log", []byte(finalSOQL), 0644)

	requestURL := fmt.Sprintf("%s/services/data/%s/query?q=%s", c.instanceURL, c.apiVersion, url.QueryEscape(finalSOQL))
	req, err := c.newRequest(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, err
	}

	var response SOQLResponse
	if _, err := c.do(req, &response); err != nil {
		return nil, err
	}

	return response.Records, nil
}

// BatchUpdateOpportunityRefs performs a batch update using the Salesforce Composite API.
func (c *Client) BatchUpdateOpportunityRefs(ctx context.Context, reference string, ids []string) error {
	if len(ids) > 25 {
		return fmt.Errorf("cannot update more than 25 records in a single batch")
	}

	batchRequest := BatchRequest{
		AllOrNone: false,
		Requests:  make([]Subrequest, len(ids)),
	}

	for i, id := range ids {
		batchRequest.Requests[i] = Subrequest{
			Method: "PATCH",
			URL:    fmt.Sprintf("/services/data/%s/sobjects/Opportunity/%s", c.apiVersion, id),
			Body:   map[string]string{"Payout_Reference__c": reference},
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

	for _, result := range response.Results {
		if result.StatusCode < 200 || result.StatusCode >= 300 {
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

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	_ = os.WriteFile("salesforce_response.json", body, 0644)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	if v != nil {
		if err := json.Unmarshal(body, v); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return resp, nil
}
