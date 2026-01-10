package main

import (
	"errors"
	"fmt"
	"net/url"
	"testing"
)

func TestPagination(t *testing.T) {

	tests := []struct {
		name           string
		inputURL       string
		pageLen        int
		totalRecordsNo int
		nextURL        string
		previousURL    string
		err            error
	}{
		{
			name:           "valid next and previous pages",
			inputURL:       "?status=ok&page=2&something=there",
			pageLen:        5,
			totalRecordsNo: 13,
			nextURL:        "?page=3&something=there&status=ok",
			previousURL:    "?page=1&something=there&status=ok",
		},
		{
			name:           "same next and previous pages",
			inputURL:       "?status=ok&page=1&something=there",
			pageLen:        5,
			totalRecordsNo: 5,
			nextURL:        "?page=1&something=there&status=ok",
			previousURL:    "?page=1&something=there&status=ok",
		},
		{
			name:           "invalid page length",
			inputURL:       "?status=ok&page=1&something=there",
			pageLen:        -5,
			totalRecordsNo: 5,
			err:            ErrInvalidPageLen,
		},
		{
			name:           "invalid page number",
			inputURL:       "?status=ok&page=4&something=there",
			pageLen:        5,
			totalRecordsNo: 14,
			err:            ErrInvalidPageNo{4, 3},
		},
	}

	for ii, tt := range tests {
		t.Run(fmt.Sprintf("%d_%s", ii, tt.name), func(t *testing.T) {

			parsedURL, err := url.Parse(tt.inputURL)
			if err != nil {
				t.Fatalf("could not parse inputURL: %v", err)
			}
			pg, err := NewPagination(tt.pageLen, tt.totalRecordsNo, parsedURL.Query())
			if err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("could not parse inputURL: %v", err)
				}
				t.Log("err", err)
				return
			}
			if err == nil && tt.err != nil {
				t.Fatalf("expected error: %v", tt.err)
			}

			if got, want := pg.NextURL(), tt.nextURL; got != want {
				t.Errorf("next url error:\ngot  %q\nwant %q", got, want)
			}
			if got, want := pg.PreviousURL(), tt.previousURL; got != want {
				t.Errorf("prev url error:\ngot  %q\nwant %q", got, want)
			}
		})
	}
}
