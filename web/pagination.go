package web

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// Pagination is a pagination helper type for use on listing pages.
type Pagination struct {
	pageLen   int
	queryVals url.Values

	PageNo   int
	Pages    int
	Next     int // 0 means no next page
	Previous int // 0 means no previous page
}

var ErrInvalidPageLen error = errors.New("pageLen cannot be below 1")

type ErrInvalidPageNo struct {
	PageNo     int
	TotalPages int
}

func (e ErrInvalidPageNo) Error() string {
	return fmt.Sprintf("invalid page number: %d (total pages: %d)", e.PageNo, e.TotalPages)
}

// NewPagination calculates the pagination setting for the provided
// pageLen (number of items to show per page), the current page number,
// the total records in the current set and the present url values. The
// url values are used for determining the url (if any) for the "Next"
// and "Previous" pages.
func NewPagination(pageLen, totalRecords, currentPage int, query url.Values) (*Pagination, error) {

	if pageLen <= 0 {
		pageLen = 1
	}

	totalPages := 1
	if totalRecords > 0 {
		totalPages = ((totalRecords - 1) / pageLen) + 1
	}

	if currentPage < 1 {
		currentPage = 1
	}
	if currentPage > totalPages {
		return nil, ErrInvalidPageNo{PageNo: currentPage, TotalPages: totalPages}
	}
	pg := &Pagination{
		pageLen:   pageLen,
		queryVals: query,
		PageNo:    currentPage,
		Pages:     totalPages,
	}

	if pg.PageNo > 1 {
		pg.Previous = pg.PageNo - 1
	}

	if pg.PageNo < pg.Pages {
		pg.Next = pg.PageNo + 1
	}

	return pg, nil
}

// buildURL generates a URL query string for a specific page.
func (p *Pagination) buildURL(page int) string {
	newQuery := make(url.Values, len(p.queryVals))
	for k, v := range p.queryVals {
		newQuery[k] = v
	}

	newQuery.Set("page", strconv.Itoa(page))
	return "?" + newQuery.Encode()
}

// NextURL returns the URL for the next page. Returns empty string if no next page.
func (p *Pagination) NextURL() string {
	if p.Next == 0 {
		return ""
	}
	return p.buildURL(p.Next)
}

// PreviousURL returns the URL for the previous page. Returns empty string if no prev page.
func (p *Pagination) PreviousURL() string {
	if p.Previous == 0 {
		return ""
	}
	return p.buildURL(p.Previous)
}
