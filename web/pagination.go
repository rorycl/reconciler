package main

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// Pagination is a pagination helper type for use on listing pages.
type Pagination struct {
	queryVals url.Values
	pageLen   int

	PageNo   int
	Pages    int
	Next     int // 0 means no next page
	Previous int // 0 means no previous page
}

// pageCount determines the number of pages in a total record set.
func (p *Pagination) pageCount(recNo int) int {
	return ((recNo - 1) / p.pageLen) + 1
}

var ErrInvalidPageLen error = errors.New("pageLen cannot be below 1")

type ErrInvalidPageNo struct {
	pageNo     int
	totalPages int
}

func (i ErrInvalidPageNo) Error() string {
	return fmt.Sprintf("page %d more than total pages %d", i.pageNo, i.totalPages)
}

// NewPagination calculates the pagination setting for the provided
// pageLen (number of items to show per page), the current page number,
// the total records in the current set and the present url values. The
// url values are used for determining the url (if any) for the "Next"
// and "Previous" pages. Consumers may wish to ignore the error as a
// valid Pagination is always returned, but it // is wise to log it at least.
func NewPagination(pageLen, totalRecordsNo int, vals url.Values) (*Pagination, error) {

	// set default
	pg := &Pagination{
		pageLen:   pageLen,
		queryVals: vals,
		PageNo:    1,
		Pages:     1,
		Next:      0,
		Previous:  0,
	}

	if pageLen < 1 {
		return pg, ErrInvalidPageLen
	}

	var err error
	pg.PageNo, err = strconv.Atoi(pg.queryVals.Get("page"))
	if err != nil {
		pg.PageNo = 1
	}
	pg.Pages = pg.pageCount(totalRecordsNo)
	if pg.PageNo > pg.Pages {
		return pg, ErrInvalidPageNo{pg.PageNo, pg.Pages}
	}
	if pg.PageNo > 1 {
		pg.Previous = pg.PageNo - 1
	}
	if pg.PageNo < pg.Pages {
		pg.Next = pg.PageNo + 1
	}
	return pg, nil
}

func (p *Pagination) pageURL(page int) string {
	if page == 0 {
		return "?" + p.queryVals.Encode()
	}
	newQuery := url.Values{}
	for k, v := range p.queryVals {
		newQuery[k] = v
	}
	newQuery.Set("page", strconv.Itoa(page))
	return "?" + newQuery.Encode()
}

func (p *Pagination) NextURL() string {
	return p.pageURL(p.Next)
}

func (p *Pagination) PreviousURL() string {
	return p.pageURL(p.Previous)
}
