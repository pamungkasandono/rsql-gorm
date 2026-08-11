package rsql

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

func applySort(t *testing.T, sorts []Sort, model any, dest any) (*gorm.DB, string, []any) {
	t.Helper()
	db := newDryRunDB(t)

	out, err := ApplySort(db, sorts, model)
	if err != nil {
		t.Fatalf("ApplySort: %v", err)
	}

	out.Find(dest)
	return out, out.Statement.SQL.String(), out.Statement.Vars
}

func TestApplySortEmpty(t *testing.T) {
	db := newDryRunDB(t)
	out, err := ApplySort(db, nil, qbUser{})
	if err != nil {
		t.Fatalf("empty sorts should not error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil *gorm.DB")
	}
}

func TestApplySortTopLevel(t *testing.T) {
	_, sql, _ := applySort(t, []Sort{{Field: "name"}}, qbUser{}, &qbUser{})
	assertContains(t, sql, "ORDER BY qb_users.name ASC")

	_, sql, _ = applySort(t, []Sort{{Field: "name", Desc: true}}, qbUser{}, &qbUser{})
	assertContains(t, sql, "ORDER BY qb_users.name DESC")
}

func TestApplySortMultiple(t *testing.T) {
	sorts := []Sort{
		{Field: "name", Desc: true},
		{Field: "CreatedAt"},
	}
	_, sql, _ := applySort(t, sorts, qbUser{}, &qbUser{})

	assertContains(t, sql, "ORDER BY qb_users.name DESC")
	assertContains(t, sql, "qb_users.created_at ASC")
}

func TestApplySortNested(t *testing.T) {
	sorts := []Sort{{Field: "roles.Role.name", Desc: true}}
	_, sql, _ := applySort(t, sorts, qbUser{}, &qbUser{})

	assertContains(t, sql, "LEFT JOIN qb_user_roles Roles ON Roles.user_id = qb_users.id")
	assertContains(t, sql, "LEFT JOIN qb_roles Roles__Role ON Roles__Role.role_id = Roles.id")
	assertContains(t, sql, "ORDER BY Roles__Role.name DESC")
}

func TestApplySortUnknownField(t *testing.T) {
	db := newDryRunDB(t)
	_, err := ApplySort(db, []Sort{{Field: "nonexistent"}}, qbUser{})
	if err == nil {
		t.Fatal("expected error for unknown sort field")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %v", err)
	}
}

func TestApplySortUnknownNestedField(t *testing.T) {
	db := newDryRunDB(t)
	_, err := ApplySort(db, []Sort{{Field: "roles.nonexistent"}}, qbUser{})
	if err == nil {
		t.Fatal("expected error for unknown nested sort field")
	}
}

func TestApplyPagination(t *testing.T) {
	db := newDryRunDB(t)
	out := ApplyPagination(db, Pagination{Page: 3, Limit: 25})
	out.Find(&qbUser{})
	sql := out.Statement.SQL.String()

	assertContains(t, sql, "LIMIT 25 OFFSET 50")
}

func TestBuildQueryWithParams(t *testing.T) {
	db := newDryRunDB(t)
	p := &Params{
		Pagination: Pagination{Page: 2, Limit: 10},
		Filter:     &ComparisonNode{Selector: "name", Operator: "==", Arguments: "John"},
		Sorts:      []Sort{{Field: "name", Desc: true}},
	}

	out, err := BuildQueryWithParams(db, p, qbUser{})
	if err != nil {
		t.Fatalf("BuildQueryWithParams: %v", err)
	}

	out.Find(&qbUser{})
	sql := out.Statement.SQL.String()

	assertContains(t, sql, "WHERE qb_users.name = ?")
	assertContains(t, sql, "ORDER BY qb_users.name DESC")
	assertContains(t, sql, "LIMIT 10 OFFSET 10")
}

func TestBuildQueryWithParamsInvalidSort(t *testing.T) {
	db := newDryRunDB(t)
	p := &Params{
		Sorts: []Sort{{Field: "nonexistent"}},
	}
	_, err := BuildQueryWithParams(db, p, qbUser{})
	if err == nil {
		t.Fatal("expected error for invalid sort field")
	}
}

func TestBuildPageableQueryReturnsSanitizedPagination(t *testing.T) {
	db := newDryRunDB(t)
	p := &Params{
		Pagination: Pagination{Page: 3, Limit: 25},
		Filter:     &ComparisonNode{Selector: "name", Operator: "==", Arguments: "John"},
		Sorts:      []Sort{{Field: "name", Desc: true}},
	}

	pq, err := BuildPageableQuery(db, p, qbUser{})
	if err != nil {
		t.Fatalf("BuildPageableQuery: %v", err)
	}
	if pq.Page != 3 || pq.Limit != 25 || pq.Offset != 50 {
		t.Errorf("sanitized pagination wrong: %+v", pq)
	}

	q := pq.NewQuery()
	q.Find(&qbUser{})
	sql := q.Statement.SQL.String()
	if strings.Contains(sql, "LIMIT") || strings.Contains(sql, "OFFSET") {
		t.Errorf("pageable query must not carry LIMIT/OFFSET, got: %s", sql)
	}
	assertContains(t, sql, "WHERE qb_users.name = ?")
	assertContains(t, sql, "ORDER BY qb_users.name DESC")
}

func TestBuildPageableQueryClampsPagination(t *testing.T) {
	db := newDryRunDB(t)
	p := &Params{
		Pagination: Pagination{Page: 0, Limit: 99999},
	}

	pq, err := BuildPageableQuery(db, p, qbUser{})
	if err != nil {
		t.Fatalf("BuildPageableQuery: %v", err)
	}
	if pq.Page != 1 {
		t.Errorf("page: want 1, got %d", pq.Page)
	}
	if pq.Limit != DefaultPaginationConfig().MaxLimit {
		t.Errorf("limit: want %d, got %d", DefaultPaginationConfig().MaxLimit, pq.Limit)
	}
}

func TestBuildPageableQueryCountThenFind(t *testing.T) {
	db := newDryRunDB(t)
	p := &Params{
		Pagination: Pagination{Page: 2, Limit: 10},
		Filter:     &ComparisonNode{Selector: "name", Operator: "==", Arguments: "John"},
	}

	pq, err := BuildPageableQuery(db, p, qbUser{})
	if err != nil {
		t.Fatalf("BuildPageableQuery: %v", err)
	}

	clauses := pq.NewQuery().Statement.Clauses
	if _, ok := clauses["LIMIT"]; ok {
		t.Error("query must not carry LIMIT clause before count")
	}
	if _, ok := clauses["OFFSET"]; ok {
		t.Error("query must not carry OFFSET clause before count")
	}

	var total int64
	if err := pq.NewQuery().Count(&total).Error; err != nil {
		t.Fatalf("count: %v", err)
	}

	find := pq.NewQuery().Limit(pq.Limit).Offset(pq.Offset)
	find.Find(&[]qbUser{})
	findSQL := find.Statement.SQL.String()
	assertContains(t, findSQL, "WHERE qb_users.name = ?")
	assertContains(t, findSQL, "LIMIT 10 OFFSET 10")
}
