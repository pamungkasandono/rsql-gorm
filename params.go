package rsql

import "strings"

// Params carries a parsed filter, sort and pagination for list/table
// endpoints. Build it with ParseListParams for the HTTP query-params flow, or
// construct it directly from a parsed Node, []Sort and Pagination when
// building requests programmatically.
type Params struct {
	Pagination Pagination
	Filter     Node
	Sorts      []Sort
}

// Pagination holds the requested page and page size. sanitize clamps them to
// the DefaultLimit/MaxLimit and MaxPage bounds before use.
type Pagination struct {
	Page  int
	Limit int
}

// Pagination bounds enforced by Pagination.sanitize.
const (
	// DefaultLimit is used when Limit is unset or below 1.
	DefaultLimit = 10
	// MaxLimit clamps Limit.
	MaxLimit = 1000
	// MaxPage clamps Page (the resulting OFFSET).
	MaxPage = 10000
)

// sanitize clamps page and limit to the DefaultLimit/MaxLimit/MaxPage bounds
// and returns the effective page, limit and offset.
func (p Pagination) sanitize() (page, limit, offset int) {
	if p.Limit < 1 {
		p.Limit = DefaultLimit
	}
	if p.Limit > MaxLimit {
		p.Limit = MaxLimit
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Page > MaxPage {
		p.Page = MaxPage
	}
	return p.Page, p.Limit, (p.Page - 1) * p.Limit
}

// Sort is a single ORDER BY term. Field is a Go struct field name (dotted
// paths are allowed) and Desc selects descending order.
type Sort struct {
	Field string
	Desc  bool
}

// ParseSort parses a comma-separated sort string like "field:desc,field2:asc"
// into []Sort. An empty string returns nil. A missing direction defaults to
// ascending; the direction token is matched case-insensitively.
func ParseSort(raw string) ([]Sort, error) {
	if raw == "" {
		return nil, nil
	}
	var sorts []Sort
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces := strings.SplitN(part, ":", 2)
		field := pieces[0]
		desc := false
		if len(pieces) == 2 {
			desc = strings.ToLower(pieces[1]) == "desc"
		}
		sorts = append(sorts, Sort{Field: field, Desc: desc})
	}
	return sorts, nil
}
