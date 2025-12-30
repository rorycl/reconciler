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

	"sfcli/config"
)

// Client is a wrapper for making authenticated calls to the Salesforce API.
type Client struct {
	httpClient  *http.Client
	instanceURL string
	apiVersion  string
	config      config.SalesforceConfig
}

// GetOpportunities fetches records from Salesforce using a configurable SOQL query.
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

	// Salesforce sobject queries provide at most 2000 records in a batch. Subsequent
	// query paths are represented by response.NextRecordsURL. If there are no more
	// records to retrieve this URL will be empty and response.Done will be true.
	// See https://developer.salesforce.com/docs/atlas.en-us.api_rest.meta/api_rest/dome_query.htm
	// and https://help.salesforce.com/s/articleView?id=000386264&type=1 for more info.
	//
	// requestURL is the initial url.
	requestURL := fmt.Sprintf("%s/services/data/%s/query?q=%s", c.instanceURL, c.apiVersion, url.QueryEscape(finalSOQL))
	var records []Record
	var pageNo int
	for {
		pageNo++
		req, err := c.newRequest(ctx, "GET", requestURL, nil)
		if err != nil {
			return nil, fmt.Errorf("newRequest error pageNo %d: %w", pageNo, err)
		}

		var response SOQLResponse
		if _, err := c.do(req, &response); err != nil {
			return nil, fmt.Errorf("soql do error pageNo %d: %w", pageNo, err)
		}
		records = append(records, response.Records...)
		if response.Done || response.NextRecordsURL == "" {
			break
		}
		requestURL, err = url.JoinPath(c.instanceURL, response.NextRecordsURL)
		if err != nil {
			return nil, fmt.Errorf("url construction error for page %d: (%s) %w", pageNo+1, response.NextRecordsURL, err)
		}

	}
	return records, nil
}

// BatchUpdateOpportunityRefs performs a update using the Salesforce
// sObject Collections API (which is a synchronous API) for up to 200
// records at a time.
// See https://developer.salesforce.com/docs/atlas.en-us.api_rest.meta/api_rest/resources_sobject_describe.htm
func (c *Client) BatchUpdateOpportunityRefs(ctx context.Context, reference string, ids []string) error {

	urlTpl := "%s/services/data/%s/composite/sobjects"

	if len(ids) > 200 {
		return fmt.Errorf("cannot update more than 200 records in a single batch")
	}

	// Build a slice of records.
	recordsForUpdate := make([]map[string]any, len(ids))
	for i, id := range ids {
		recordsForUpdate[i] = map[string]any{
			"id":                      id,
			c.config.LinkingFieldName: reference,
			"attributes":              map[string]string{"type": c.config.LinkingObject},
		}
	}

	// Wrap records in the required request body structure.
	payload := CollectionsUpdateRequest{
		AllOrNone: true,
		Records:   recordsForUpdate,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal batch request: %w", err)
	}
	_ = os.WriteFile("batchRequest", body, 0644)

	requestURL := fmt.Sprintf(urlTpl, c.instanceURL, c.apiVersion)
	req, err := c.newRequest(ctx, "PATCH", requestURL, body)
	if err != nil {
		return err
	}

	var response CollectionsUpdateResponse
	if _, err := c.do(req, &response); err != nil {
		return err
	}

	// Check for errors within the response array.
	var errorMessages []string
	for _, result := range response {
		if !result.Success {
			var errors []string
			for _, e := range result.Errors {
				errors = append(errors, fmt.Sprintf("%s (%s)", e.Message, e.ErrorCode))
			}
			msg := fmt.Sprintf("failed to update record %s: %s", result.ID,
				strings.Join(errors, ", "))
			errorMessages = append(errorMessages, msg)
		}
	}

	if len(errorMessages) > 0 {
		return fmt.Errorf("one or more records failed to update:\n- %s",
			strings.Join(errorMessages, "\n- "))
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
