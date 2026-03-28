package main

import (
	"log/slog"
	"testing"

	"github.com/rorycl/reconciler/config"
	"github.com/rorycl/reconciler/internal/token"
)

func TestValueStorer(t *testing.T) {

	et := &token.ExtendedToken{
		Type: token.SalesforceToken,
	}

	vs := newValueStorer()
	ctx := t.Context()
	vs.Put(ctx, "hi", "there")

	if got, want := vs.GetString(ctx, "hi"), "there"; got != want {
		t.Errorf("got %s expected %s", got, want)
	}
	vs.Remove(ctx, "hi")
	if got, want := vs.GetString(ctx, "hi"), ""; got != want {
		t.Errorf("unexpected %s", got)
	}

	vs.Put(ctx, "token", et)
	token := vs.getExtendedToken("token")
	if got, want := token.Type, et.Type; got != want {
		t.Errorf("got token %v want %v", token, et)
	}

}

func TestSFClientMaker(t *testing.T) {

	et := &token.ExtendedToken{
		Type: token.SalesforceToken,
	}
	cfg, err := config.Load("../../config/config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}

	_, err = sfClientMaker(t.Context(), cfg, slog.Default(), et)
	if err != nil {
		t.Fatal(err)
	}

	// self-evident.
	// if _, ok := sf.(sfClienter); !ok {
	// 	t.Errorf("got type %T from sfClientMaker call", sf)
	// }

}
