package rsql

import "strings"

type Params struct {
	Pagination Pagination
	Filter     Node
	Sorts      []Sort
}

type Pagination struct {
	Page  int
	Limit int
}

const (
	DefaultLimit = 10
	MaxLimit     = 1000
)

func (p Pagination) Sanitize() (page, limit, offset int) {
	if p.Limit < 1 {
		p.Limit = DefaultLimit
	}
	if p.Limit > MaxLimit {
		p.Limit = MaxLimit
	}
	if p.Page < 1 {
		p.Page = 1
	}
	return p.Page, p.Limit, (p.Page - 1) * p.Limit
}

type Sort struct {
	Field string
	Desc  bool
}

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
