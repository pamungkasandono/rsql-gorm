package rsql

import (
	"fmt"
	"strconv"
)

// ParseListParams parses the HTTP query-params flow: filter, sort, page and
// page size as strings. Page and page size must be digits only (empty means
// the default); any other value returns an error instead of silently
// clamping.
func ParseListParams(filter, sort, pageStr, pageSizeStr string) (*Params, error) {
	var p Params
	var err error

	filterNode, err := Parse(filter)
	if err != nil {
		return nil, err
	}
	p.Filter = filterNode

	p.Sorts, err = ParseSort(sort)
	if err != nil {
		return nil, err
	}

	p.Pagination.Page, err = parseNumericParam(pageStr, "page", 1)
	if err != nil {
		return nil, err
	}
	p.Pagination.Limit, err = parseNumericParam(pageSizeStr, "page size", currentConfig().DefaultLimit)
	if err != nil {
		return nil, err
	}

	return &p, nil
}

// parseNumericParam parses a query param that must be digits only (empty means
// fallback). Non-numeric input returns an error instead of silently clamping.
func parseNumericParam(s, name string, fallback int) (int, error) {
	if s == "" {
		return fallback, nil
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("pagination: %s %q must be numeric", name, s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("pagination: %s %q must be numeric", name, s)
	}
	return n, nil
}
