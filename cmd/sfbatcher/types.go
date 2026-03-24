package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/rorycl/reconciler/apiclients/salesforce"
	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/internal/token"
)

// valueStorer is an implementation of the main functions of a storage implementation,
// utilised by the token web client. In a webserver context the token.ValueStorer is met
// by a session store.
type valueStorer struct {
	data map[string]any
}

func newValueStorer() *valueStorer {
	return &valueStorer{
		data: map[string]any{},
	}
}

func (vs *valueStorer) Put(ctx context.Context, key string, val any) {
	vs.data[key] = val
}
func (vs *valueStorer) Remove(ctx context.Context, key string) {
	delete(vs.data, key)
}
func (vs *valueStorer) GetString(ctx context.Context, key string) string {
	if _, ok := vs.data[key]; !ok {
		return ""
	}
	str, ok := vs.data[key].(string)
	if !ok {
		return ""
	}
	return str
}

// getExtendedToken retrieves a token from the valueStorer map.
func (vs *valueStorer) getExtendedToken(key string) *token.ExtendedToken {
	et, ok := vs.data[key]
	if !ok {
		return nil
	}
	etT := et.(*token.ExtendedToken)
	if !ok {
		return nil
	}
	return etT
}

// sfClienter is an interface to the BatchUpdateOpportunityRefs component of the
// salesforce.Client.
type sfClienter interface {
	BatchUpdateOpportunityRefs(ctx context.Context, idRefs []salesforce.IDRef, allOrNone bool) (salesforce.CollectionsUpdateResponse, error)
}

// sfClientMaker is an adapter from salesforce.NewClient to an sfClienter.
func sfClientMaker(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (sfClienter, error) {
	return salesforce.NewClient(ctx, cfg, logger, et)
}

// sfClientMakerFunc is the type of the sfClienter initialiser func.
type sfClientMakerFunc func(ctx context.Context, cfg *config.Config, logger *slog.Logger, et *token.ExtendedToken) (sfClienter, error)

// oauth2Agent is an interface to the internal/token OAuth2 methods.
type oauth2Agent interface {
	InitiateLogin(ctx context.Context) (string, error)
	WebLoginCallBack() func(w http.ResponseWriter, r *http.Request) error
}
