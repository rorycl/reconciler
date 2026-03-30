package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/internal/token"
)

// batchSize is the number of records to send to Salesforce for updating in a batch.
// See https://developer.salesforce.com/docs/atlas.en-us.api_rest.meta/api_rest/resources_composite_sobjects_collections_update.htm
// for more information
const batchSize = 200

// runner holds the information required to parse Excel data, then login to salesforce
// and run a batch update of the targeted records.
type runner struct {
	data           *Data
	tokenType      token.TokenType
	cfg            *config.Config
	serverAddress  string
	log            *slog.Logger
	vs             *valueStorer
	sfClientMaker  sfClientMakerFunc
	loginAgent     oauth2Agent
	connectTimeout time.Duration
}

// newRunner creates a runner. Please refer to types.go for information about the
// sfClientMakerFunc, a factory func, and the loginAgent interface, both of which have
// sensible default if provided as nil values. The action defines either a "link" action
// or "unlink", the former to add a reference to salesforce records, the latter to
// remove one by setting the field to an empty string.
func newRunner(
	filename string,
	action string,
	cfg *config.Config,
	logger *slog.Logger,
	sfMaker sfClientMakerFunc,
	loginAgent oauth2Agent,
) (*runner, error) {

	switch action {
	case "link", "unlink":
	default:
		return nil, fmt.Errorf("got invalid action %q, must be 'link' or 'unlink'", action)
	}

	r := &runner{
		tokenType:      token.SalesforceToken,
		cfg:            cfg,
		serverAddress:  cfg.Web.ListenAddress,
		log:            logger,
		connectTimeout: 60 * time.Second,
	}

	// Run the parser.
	parser, err := NewExcelParser(filename)
	if err != nil {
		return r, err
	}

	// Validate the data.
	r.data, err = NewData(parser, action)
	if err != nil {
		return r, err
	}

	if sfMaker == nil {
		r.sfClientMaker = sfClientMaker
	} else {
		r.sfClientMaker = sfMaker
	}

	// Initialise the local store (in other contexts, a session store)
	r.vs = newValueStorer()

	// Initalise the token web client.
	if loginAgent == nil {
		r.loginAgent, err = token.NewTokenWebClient(
			r.tokenType,
			r.cfg.Salesforce.OAuth2Config,
			r.vs,
		)
		if err != nil {
			return r, err
		}
	} else {
		r.loginAgent = loginAgent
	}

	return r, nil

}

// run runs the runner, taking the user through the interactive login flow and on
// success proceeding with the batch update process.
func (r *runner) run() error {

	// Context create.
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
	}()

	// Generate the start of the OAuth 2 flow for Salesforce.
	authURL, err := r.loginAgent.InitiateLogin(ctx)
	if err != nil {
		return err
	}

	// Run the web server in a go routine until it times out or returns.
	errChan := make(chan error)

	// Instantiate the server.
	webServer := http.Server{
		Addr: r.serverAddress,
	}

	go func() {

		// webConnectWrapper wraps an appHandler such as token.WebLoginCallBack, which
		// has the signature:
		//
		//	func(http.ResponseWriter, *http.Request) error
		//
		// The result is put on the errChan.
		webConnectWrapper := func(h func(http.ResponseWriter, *http.Request) error) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				errChan <- h(w, r)
			})
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/connected", func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, "Connection successful. You can now close this tab.")
		})
		mux.Handle(r.cfg.Web.SalesforceCallBack, webConnectWrapper(
			r.loginAgent.WebLoginCallBack("/connected"), // connected is the redirect target.
		))
		webServer.Handler = mux

		// Ask the user to visit Salesforce to complete the oauth2 flow.
		fmt.Printf("Please login to Salesforce and authorize this batch update:\n%s\n", authURL)

		err := webServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- fmt.Errorf("server exit error: %w", err)
		}
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond) // allow time for redirection to /connected
		r.log.Info("connection succesfully made")
		_ = webServer.Close()
		close(errChan)
		break
	case <-time.After(r.connectTimeout):
		_ = webServer.Close()
		close(errChan)
		return fmt.Errorf("oauth2 connection timed out after %s seconds", r.connectTimeout)
	}

	// Initialise the salesforce client with the token.
	extendedToken := r.vs.getExtendedToken(r.tokenType.SessionName())
	if extendedToken == nil {
		return errors.New("could not extract token from store")
	}
	sfClient, err := r.sfClientMaker(ctx, r.cfg, r.log, extendedToken)
	if err != nil {
		return err
	}

	// Run the batch update by iterating over the data in batches.
	allOrNone := true
	batchCount := 1
	for idRefsBatch := range r.data.Batch(batchSize) {
		r.log.Info("running batch", "count", batchCount)
		_, err := sfClient.BatchUpdateOpportunityRefs(ctx, idRefsBatch, allOrNone)
		if err != nil {
			return fmt.Errorf("batch %d (1 indexed) update error: %w", batchCount, err)
		}
		batchCount++
	}
	r.log.Info("batch completed successfully", "records", r.data.Records)

	return nil

}
