package token

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/oauth2"
)

// TokenType is the type of an OAuth2 decorated Token.
type TokenType int

const (
	NoneToken TokenType = iota
	XeroToken
	SalesforceToken
)

var tokenName = map[TokenType]string{
	NoneToken:       "invalid",
	XeroToken:       "xero",
	SalesforceToken: "salesforce",
}

// String returns the TokenType name string.
func (tt TokenType) String() string {
	return tokenName[tt]
}

// ExtendedToken is an OAuth2 token with additional information.
type ExtendedToken struct {
	Type        TokenType     `json:"type"`
	Token       *oauth2.Token `json:"token"`
	InstanceURL string        `json:"instance_url"` // only relevant to Salesforce Tokens
}

// NewExtendedToken creates a new ExtendedToken, running any ancillary checks and/or
// fixups that might be necessary. Presently the Salesforce token requires the
// instance_url to be extracted, and the expiry time set manually.
func NewExtendedToken(typer TokenType, token *oauth2.Token) (*ExtendedToken, error) {
	if token == nil {
		return nil, errors.New("nil token received")
	}
	switch typer {
	case XeroToken, SalesforceToken:
	default:
		return nil, errors.New("invalid token type received")
	}

	et := &ExtendedToken{
		Type:  typer,
		Token: token,
	}

	if typer == SalesforceToken {
		err := et.fixSalesForceToken()
		if err != nil {
			return nil, fmt.Errorf("could not fix new salesforce token: %w", err)
		}
	}

	return et, nil
}

// IsValid checks if the token is valid or if a refresh token exists to get a new token.
// Tokens that expire after the expirationDuration will be considered invalid. This is
// on the assumption that the validity period of tokens AND refresh tokens is known.
func (et *ExtendedToken) IsValid(expirationDuration time.Duration) bool {
	if et.Token == nil {
		return false
	}
	if et.Token.Expiry.IsZero() {
		return false
	}
	projectedExpiry := time.Now().UTC().Add(-1 * expirationDuration)
	if !et.Token.Expiry.After(projectedExpiry) {
		return false
	}
	return et.Token.Valid() || et.Token.RefreshToken != ""
}

// fixSalesforceTokenExpiry fixes the "IsZero" expiry of Saleforce's tokens.
// sf tokens are something like the following. Annoyingly, the expiration time is set to
// the Go zero time. This causes the TokenIsValid check to fail.
//
//	&oauth2.Token{
//		AccessToken:"xxxx",
//		TokenType:"Bearer",
//		RefreshToken:"yyy",
//		Expiry:time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC),
//		ExpiresIn:0,
//		raw:map[string]interface {}{
//			"access_token":"xxx",
//			"id":"https://login.salesforce.com/id/00DxxxxxxA1/uuuuuuuuuuuuuuuuuu",
//			"instance_url":"https://orgfarm-xxxxxxxxxx-dev-ed.develop.my.salesforce.com",
//			"issued_at":"1771257689412",
//			"refresh_token":"yyy",
//			"scope":"refresh_token api",
//			"signature":"zzz",
//			"token_type":"Bearer"
//		},
//		expiryDelta:0,
//	}
func (et *ExtendedToken) fixSalesforceTokenExpiry() error {
	if et == nil {
		return errors.New("nil token in fixSalesforceTokenExpiry")
	}
	// Salesforce sends issued_at in milliseconds as a string
	issuedAtStr, ok := et.Token.Extra("issued_at").(string)
	if !ok || issuedAtStr == "" {
		return errors.New("no issued_at found in salesforce token")
	}
	ms, err := strconv.ParseInt(issuedAtStr, 10, 64)
	if err != nil {
		return fmt.Errorf("could not parse salesforce issuedAtString: %w", err)
	}
	// Set Expiry.
	const sessionLength = 2 * time.Hour
	et.Token.Expiry = time.UnixMilli(ms).Add(sessionLength)
	return nil
}

// setSalesforceInstanceURL retrieves the "instance_url" from a salesforce token. See
// https://help.salesforce.com/s/articleView?id=xcloud.remoteaccess_oauth_client_credentials_flow.htm&type=5
func (et *ExtendedToken) setSalesforceInstanceURL() error {
	if et == nil {
		return errors.New("nil token in setSalesforceInstanceURL")
	}
	var ok bool
	et.InstanceURL, ok = et.Token.Extra("instance_url").(string)
	if !ok || et.InstanceURL == "" {
		return errors.New("no instance_url found in salesforce token")
	}
	return nil
}

// fixSalesForceToken wraps up various Salesforce Token fixer functions.
func (et *ExtendedToken) fixSalesForceToken() error {
	err := et.fixSalesforceTokenExpiry()
	if err != nil {
		return err
	}
	return et.setSalesforceInstanceURL()
}

// ReuseOrRefresh attempts to use or refresh an ExtendedToken using the provided context
// and oauth2.Config. The config.TokenSource func automatically refreshes tokens when
// needed. The function returns whether refreshing occurred and any error.
func (et *ExtendedToken) ReuseOrRefresh(ctx context.Context, config *oauth2.Config) (bool, error) {
	var refreshed bool

	tok := config.TokenSource(ctx, et.Token)
	possibleNewToken, err := tok.Token()
	if err != nil {
		return refreshed, fmt.Errorf("could not reuse or refresh token: %w", err)
	}

	// Check if refreshing occured. If not, return early.
	if possibleNewToken.AccessToken == et.Token.AccessToken {
		return refreshed, nil
	}
	refreshed = true
	et.Token = possibleNewToken
	fmt.Printf("possible New Token %#v\n", possibleNewToken)

	if et.Type == SalesforceToken {
		if err := et.fixSalesForceToken(); err != nil {
			return refreshed, fmt.Errorf("could not fix reused/new salesforce token: %w", err)
		}
	}
	return refreshed, nil
}
