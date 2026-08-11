package rsql

import (
	"fmt"
	"strings"
	"sync/atomic"
)

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
// the current PaginationConfig bounds before use.
type Pagination struct {
	Page  int
	Limit int
}

// PaginationConfig holds the process-wide pagination bounds used by
// BuildPageableQuery, ApplyPagination and ParseListParams. The zero value is
// invalid; use DefaultPaginationConfig or build one explicitly before calling
// SetPaginationConfig.
type PaginationConfig struct {
	DefaultLimit int
	MaxLimit     int
	MaxPage      int
}

// paginationConfig holds the effective bounds; reads are race-free via atomic.
var paginationConfig atomic.Pointer[PaginationConfig]

func init() {
	cfg := DefaultPaginationConfig()
	paginationConfig.Store(&cfg)
}

// DefaultPaginationConfig returns the built-in bounds: default limit 10, max
// limit 1000, max page 10000.
func DefaultPaginationConfig() PaginationConfig {
	return PaginationConfig{DefaultLimit: 10, MaxLimit: 1000, MaxPage: 10000}
}

// CurrentPaginationConfig returns the currently effective bounds.
func CurrentPaginationConfig() PaginationConfig {
	return currentConfig()
}

// SetPaginationConfig replaces the effective bounds process-wide and returns
// an error if the config is invalid. Call it once at startup; it is safe to
// call concurrently with queries.
func SetPaginationConfig(cfg PaginationConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	paginationConfig.Store(&cfg)
	return nil
}

// currentConfig returns the effective bounds without copying through a public
// API. Used by sanitize and ParseListParams on the hot path.
func currentConfig() PaginationConfig {
	return *paginationConfig.Load()
}

func (c PaginationConfig) validate() error {
	if c.DefaultLimit < 1 {
		return fmt.Errorf("pagination: DefaultLimit must be >= 1, got %d", c.DefaultLimit)
	}
	if c.MaxLimit < c.DefaultLimit {
		return fmt.Errorf("pagination: MaxLimit %d must be >= DefaultLimit %d", c.MaxLimit, c.DefaultLimit)
	}
	if c.MaxPage < 1 {
		return fmt.Errorf("pagination: MaxPage must be >= 1, got %d", c.MaxPage)
	}
	return nil
}

// sanitize clamps page and limit to the current PaginationConfig bounds and
// returns the effective page, limit and offset.
func (p Pagination) sanitize() (page, limit, offset int) {
	cfg := currentConfig()
	if p.Limit < 1 {
		p.Limit = cfg.DefaultLimit
	}
	if p.Limit > cfg.MaxLimit {
		p.Limit = cfg.MaxLimit
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Page > cfg.MaxPage {
		p.Page = cfg.MaxPage
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
