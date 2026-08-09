package rsql

import "strconv"

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

	p.Pagination.Page, _ = strconv.Atoi(defaultIfEmpty(pageStr, "1"))
	p.Pagination.Limit, _ = strconv.Atoi(defaultIfEmpty(pageSizeStr, "10"))

	return &p, nil
}

func defaultIfEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
