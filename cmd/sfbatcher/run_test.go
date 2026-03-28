package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/internal/token"
	"golang.org/x/oauth2"
)

// sfTest meets the sfClienter interface.
type sfTest struct {
	batches int
	records int
}

func (sf *sfTest) BatchUpdateOpportunityRefs(ctx context.Context, idRefs []salesforce.IDRef, allOrNone bool) (salesforce.CollectionsUpdateResponse, error) {
	sf.batches++
	sf.records += len(idRefs)
	return nil, nil
}

// testSFMaker is a factory func to make an sfClienter via sfTest.
func testSFMaker(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (sfClienter, error) {
	return &sfTest{}, nil
}

// loginService provides the methods of the oauth2Agent interface to internal/token
// OAuth2 methods.
type testLoginService struct {
	sessionKey string
	et         *token.ExtendedToken
	vs         *valueStorer
}

func (ts *testLoginService) InitiateLogin(ctx context.Context) (string, error) {
	return "ok", nil
}
func (ts *testLoginService) WebLoginCallBack() func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		ts.vs.Put(r.Context(), ts.sessionKey, ts.et)
		return nil
	}
}

func TestRunner(t *testing.T) {

	config, err := config.Load("../../config/config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	config.Web.ListenAddress = "127.0.0.1:8089"

	testLoginAgent := &testLoginService{
		sessionKey: token.SalesforceToken.SessionName(),
		et: &token.ExtendedToken{
			Type:        token.SalesforceToken,
			InstanceURL: config.Salesforce.OAuth2Config.RedirectURL,
			Token: &oauth2.Token{
				AccessToken: "valid-token-123",
				Expiry:      time.Now().Add(1 * time.Hour), // not expired
			},
		},
		vs: newValueStorer(),
	}

	runner, err := newRunner(
		"testdata/valid.xlsx",
		"link",
		config,
		slog.Default(),
		testSFMaker,
		testLoginAgent,
	)
	if err != nil {
		t.Fatal(err)
	}

	// override runner values for testing
	runner.connectTimeout = 10 * time.Millisecond
	runner.vs = testLoginAgent.vs

	// Run the runner in a goroutine since it depends on a web callback.
	errorChan := make(chan error)
	go func() {
		defer func() {
			close(errorChan)
		}()

		// Run the process.
		err = runner.run()
		if err != nil {
			errorChan <- fmt.Errorf("runner.run error: %v", err)
			return
		}

		// Check the number of processed records.
		if got, want := runner.data.Records, 3; got != want {
			errorChan <- fmt.Errorf("got %d want %d records", got, want)
			return
		}

	}()

	// Run an http client to trigger the callback, and continue processing.
	go func(t *testing.T) {
		time.Sleep(3 * time.Millisecond) // wait for the server to spin up
		resp, err := http.Get("http://" + config.Web.ListenAddress + "/" + config.Web.SalesforceCallBack)
		if err != nil {
			errorChan <- fmt.Errorf("http get error: %v", err)
		}
		if resp.StatusCode != 200 {
			errorChan <- fmt.Errorf("http status != 200: %d", resp.StatusCode)
		}
	}(t)

	err = <-errorChan
	if err != nil {
		t.Fatal(err)
	}

}
