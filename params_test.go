package rsql

import "testing"

func TestParseSort(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		field string
		desc  bool
	}{
		{"asc", "name:asc", "name", false},
		{"desc", "created_at:desc", "created_at", true},
		{"no_direction", "name", "name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sorts, err := ParseSort(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(sorts) != 1 {
				t.Fatalf("expected 1 sort, got %d", len(sorts))
			}
			if sorts[0].Field != tt.field {
				t.Errorf("field: want %q, got %q", tt.field, sorts[0].Field)
			}
			if sorts[0].Desc != tt.desc {
				t.Errorf("desc: want %v, got %v", tt.desc, sorts[0].Desc)
			}
		})
	}
}

func TestParseSortEmpty(t *testing.T) {
	sorts, err := ParseSort("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sorts != nil {
		t.Fatalf("expected nil for empty input, got %v", sorts)
	}
}

func TestParseSortMultiple(t *testing.T) {
	sorts, err := ParseSort("name:desc,created_at:asc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorts) != 2 {
		t.Fatalf("expected 2 sorts, got %d", len(sorts))
	}
	if sorts[0].Field != "name" || !sorts[0].Desc {
		t.Errorf("first sort wrong: %+v", sorts[0])
	}
	if sorts[1].Field != "created_at" || sorts[1].Desc {
		t.Errorf("second sort wrong: %+v", sorts[1])
	}
}

func TestPaginationSanitize(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		limit      int
		wantPage   int
		wantLimit  int
		wantOffset int
	}{
		{"defaults", 0, 0, 1, DefaultLimit, 0},
		{"valid", 3, 20, 3, 20, 40},
		{"over_max", 2, 99999, 2, MaxLimit, MaxLimit},
		{"page_over_max", MaxPage + 1000, 20, MaxPage, 20, (MaxPage - 1) * 20},
		{"negative", -1, -5, 1, DefaultLimit, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pagination{Page: tt.page, Limit: tt.limit}
			page, limit, offset := p.Sanitize()
			if page != tt.wantPage {
				t.Errorf("page: want %d, got %d", tt.wantPage, page)
			}
			if limit != tt.wantLimit {
				t.Errorf("limit: want %d, got %d", tt.wantLimit, limit)
			}
			if offset != tt.wantOffset {
				t.Errorf("offset: want %d, got %d", tt.wantOffset, offset)
			}
		})
	}
}

func TestParseListParams(t *testing.T) {
	p, err := ParseListParams("status==ACTIVE", "name:desc", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p.Filter == nil {
		t.Fatal("expected non-nil filter")
	}
	cmp, ok := p.Filter.(*ComparisonNode)
	if !ok {
		t.Fatalf("expected ComparisonNode, got %T", p.Filter)
	}
	if cmp.Selector != "status" || cmp.Operator != "==" || cmp.Arguments != "ACTIVE" {
		t.Errorf("filter wrong: %+v", cmp)
	}

	if len(p.Sorts) != 1 || p.Sorts[0].Field != "name" || !p.Sorts[0].Desc {
		t.Errorf("sorts wrong: %+v", p.Sorts)
	}

	if p.Pagination.Page != 1 || p.Pagination.Limit != DefaultLimit {
		t.Errorf("default pagination wrong: %+v", p.Pagination)
	}
}

func TestParseListParamsCustomPagination(t *testing.T) {
	p, err := ParseListParams("", "", "3", "50")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Filter != nil {
		t.Errorf("expected nil filter for empty input, got %v", p.Filter)
	}
	if p.Pagination.Page != 3 || p.Pagination.Limit != 50 {
		t.Errorf("pagination wrong: %+v", p.Pagination)
	}
}

func TestParseListParamsInvalidFilter(t *testing.T) {
	_, err := ParseListParams("invalid", "", "", "")
	if err == nil {
		t.Fatal("expected error for invalid filter")
	}
}

func TestParseListParamsNonNumericPage(t *testing.T) {
	_, err := ParseListParams("", "", "abc", "")
	if err == nil {
		t.Fatal("expected error for non-numeric page")
	}
}

func TestParseListParamsNonNumericPageSize(t *testing.T) {
	_, err := ParseListParams("", "", "", "1.5")
	if err == nil {
		t.Fatal("expected error for non-numeric page size")
	}
}

func TestParseListParamsNegativePage(t *testing.T) {
	_, err := ParseListParams("", "", "-2", "")
	if err == nil {
		t.Fatal("expected error for negative page")
	}
}
