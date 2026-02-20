package salesforce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reconciler/apiclients/token"
	"reconciler/config"

	"golang.org/x/oauth2"
)

// SalesforceAPIVersionNumber sets out the currently supported
// Salesforce API used for this client.
const SalesforceAPIVersionNumber = "v65.0"

// maxBatchUpdateCount is the maximum number of Salesforce records that
// can be updated in one operation.
const maxBatchUpdateCount = 200

// Client is a wrapper for making authenticated calls to the Salesforce API.
type Client struct {
	httpClient  *http.Client
	instanceURL string
	apiVersion  string
	config      config.Config
	log         *slog.Logger
}

// NewClient is provided a valid (refreshed where necessary) token and returns a
// Salesforce client. It is the responsibility of the caller to ensure the provided
// token is refreshed.
func NewClient(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (*Client, error) {
	oauthClient := oauth2.NewClient(ctx, et.Token)
	return &Client{
		httpClient:  oauthClient,
		instanceURL: et.InstanceURL,
		apiVersion:  SalesforceAPIVersionNumber,
		config:      *cfg,
		log:         logger,
	}, nil
}

// GetOpportunities fetches records from Salesforce using a configurable SOQL query.
func (c *Client) GetOpportunities(ctx context.Context, fromDate, ifModifiedSince time.Time) ([]Donation, error) {

	var conditions []string
	conditions = append(conditions, fmt.Sprintf("CloseDate >= %s", fromDate.Format("2006-01-02")))
	if !ifModifiedSince.IsZero() {
		conditions = append(conditions, fmt.Sprintf("LastModifiedDate > %s", ifModifiedSince.UTC().Format(time.RFC3339)))
	}
	whereClause := strings.Join(conditions, " AND ")

	// Replace the placeholder in the query template with the generated WHERE clause.
	finalSOQL := strings.Replace(c.config.Salesforce.Query, "{{.WhereClause}}", whereClause, 1)
	c.log.Info(fmt.Sprintf("GetOpportunities sql: %s", finalSOQL))

	// Dump the final query for debugging purposes.
	// _ = os.WriteFile("salesforce_query.log", []byte(finalSOQL), 0644)

	// Salesforce sobject queries provide at most 2000 records in a batch. Subsequent
	// query paths are represented by response.NextRecordsURL. If there are no more
	// records to retrieve this URL will be empty and response.Done will be true.
	// See https://developer.salesforce.com/docs/atlas.en-us.api_rest.meta/api_rest/dome_query.htm
	// and https://help.salesforce.com/s/articleView?id=000386264&type=1 for more info.
	//
	// requestURL is the initial url.
	requestURL := fmt.Sprintf("%s/services/data/%s/query?q=%s", c.instanceURL, c.apiVersion, url.QueryEscape(finalSOQL))
	var records []Donation
	var pageNo int
	for {
		pageNo++
		c.log.Debug(fmt.Sprintf("GetOpportunities: page %d: url %s", pageNo, requestURL))

		req, err := c.newRequest(ctx, "GET", requestURL, nil)
		if err != nil {
			c.log.Error(fmt.Sprintf("GetOpportunities: newRequest error pageNo %d: %v", pageNo, err))
			return nil, fmt.Errorf("newRequest error pageNo %d: %w", pageNo, err)
		}

		var response SOQLResponse
		if _, err := c.do(req, &response); err != nil {
			c.log.Error(fmt.Sprintf("GetOpportunities soql do error pageNo %d: %v", pageNo, err))
			return nil, fmt.Errorf("soql do error pageNo %d: %w", pageNo, err)
		}
		records = append(records, response.Donations...)
		if response.Done || response.NextRecordsURL == "" {
			break
		}
		requestURL, err = url.JoinPath(c.instanceURL, response.NextRecordsURL)
		if err != nil {
			c.log.Error(fmt.Sprintf("GetOpportunities: url construction error for page %d: (%s) %v", pageNo+1, response.NextRecordsURL, err))
			return nil, fmt.Errorf("url construction error for page %d: (%s) %w", pageNo+1, response.NextRecordsURL, err)
		}

	}
	return records, nil
}

// BatchUpdateOpportunityRefs performs a update using the Salesforce sObject Collections
// API (which is a synchronous API) for up to 200 records at a time. See
// https://developer.salesforce.com/docs/atlas.en-us.api_rest.meta/api_rest/resources_sobject_describe.htm.
//
// The method replaces the data in the stated salesforce LinkingFieldName for salesforce
// opportunity records with the provided IDs with the provided `reference`.
//
// Note that setting `allOrNone` to true makes the SOQL update atomic and the
// transaction will fail in it's entirety if any single opportunity record cannot be
// updated. See
// https://developer.salesforce.com/docs/atlas.en-us.api_rest.meta/api_rest/resources_composite_allornone.htm
// for more information about "allOrNone Parameters in Composite and Collections
// Requests".
func (c *Client) BatchUpdateOpportunityRefs(
	ctx context.Context,
	reference string,
	ids []string,
	allOrNone bool) (CollectionsUpdateResponse, error) {

	urlTpl := "%s/services/data/%s/composite/sobjects"

	if len(ids) > maxBatchUpdateCount {
		c.log.Error("BatchUpdateOpportunityRefs: cannot update more than 200 records in a single batch")
		return nil, fmt.Errorf("cannot update more than 200 records in a single batch")
	}

	// Build a slice of records.
	donationsForUpdate := make([]map[string]any, len(ids))
	for i, id := range ids {
		donationsForUpdate[i] = map[string]any{
			"id":                                 id,
			c.config.Salesforce.LinkingFieldName: reference,
			"attributes": map[string]string{
				"type": c.config.Salesforce.LinkingObject,
			},
		}
	}

	// Wrap records in the required request body structure.
	payload := CollectionsUpdateRequest{
		AllOrNone: allOrNone,
		Records:   donationsForUpdate,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		c.log.Error(fmt.Sprintf("BatchUpdateOpportunityRefs: failed to marshal batch request: %v", err))
		return nil, fmt.Errorf("failed to marshal batch request: %w", err)
	}

	requestURL := fmt.Sprintf(urlTpl, c.instanceURL, c.apiVersion)
	c.log.Debug(fmt.Sprintf("BatchUpdateOpportunityRefs: requestURL %s", requestURL))

	req, err := c.newRequest(ctx, "PATCH", requestURL, body)
	if err != nil {
		c.log.Error(fmt.Sprintf("BatchUpdateOpportunityRefs: new patch request error: %v", err))
		return nil, fmt.Errorf("new patch request error: %w", err)
	}

	var response CollectionsUpdateResponse
	if _, err := c.do(req, &response); err != nil {
		c.log.Error(fmt.Sprintf("BatchUpdateOpportunityRefs: response error: %v", err))
		return nil, err
	}

	// Check for errors within the response array.
	var errorMessages []string
	for _, result := range response {
		if !result.Success {
			var errors []string
			for _, e := range result.Errors {
				errors = append(errors, fmt.Sprintf("%s (%s)", e.Message, e.ErrorCode))
			}
			msg := fmt.Sprintf("BatchUpdateOpportunityRefs: failed to update donation %s: %s", result.ID, strings.Join(errors, ", "))
			errorMessages = append(errorMessages, msg)
		}
	}

	if len(errorMessages) > 0 {

		c.log.Error("BatchUpdateOpportunityRefs: one or more donations failed to update")
		return response, fmt.Errorf("one or more donations failed to update:\n- %s",
			strings.Join(errorMessages, "\n- "))
	}

	c.log.Info("BatchUpdateOpportunityRefs: one or more donations failed to update")
	return response, nil
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
func (c *Client) do(req *http.Request, v any) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Uncomment to save the raw response to disk for debugging.
	// _ = os.WriteFile("/tmp/salesforce_response.json", body, 0644)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	if v != nil {
		// check if v is of the SOQLResponse type
		soqlResponsePtr, ok := v.(*SOQLResponse)
		if !ok {
			// Return a generic response if not.
			return resp, json.Unmarshal(body, v)
		}
		// unmarshal
		unmarshaller := SOQLUnmarshaller{Mapper: c.config.Salesforce.FieldMappings}
		data, err := unmarshaller.UnmarshalSOQLResponse(body)
		if err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		// deference pointer and assign original
		*soqlResponsePtr = *data
	}

	return resp, nil
}
