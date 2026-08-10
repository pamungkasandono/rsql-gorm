package rsql

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

type aliasProduct struct {
	ID       string         `gorm:"column:id;primaryKey"`
	Name     string         `gorm:"column:name"`
	Brand    aliasBrand     `gorm:"foreignKey:BrandID;references:ID"`
	BrandID  string         `gorm:"column:brand_id"`
	Cats     []aliasProdCat `gorm:"foreignKey:ProductID;references:ID"`
	Vars     []aliasVariant `gorm:"foreignKey:ProductID;references:ID"`
	Detail   aliasDetail    `gorm:"foreignKey:DetailID;references:ID"`
	DetailID string         `gorm:"column:detail_id"`
}

func (aliasProduct) TableName() string { return "alias_products" }

type aliasBrand struct {
	ID   string `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

func (aliasBrand) TableName() string { return "alias_brands" }

type aliasProdCat struct {
	ID         string        `gorm:"column:id;primaryKey"`
	ProductID  string        `gorm:"column:product_id"`
	CategoryID string        `gorm:"column:category_id"`
	Category   aliasCategory `gorm:"foreignKey:CategoryID;references:ID"`
}

func (aliasProdCat) TableName() string { return "alias_product_categories" }

type aliasCategory struct {
	ID   string `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

func (aliasCategory) TableName() string { return "alias_categories" }

type aliasVariant struct {
	ID        string `gorm:"column:id;primaryKey"`
	ProductID string `gorm:"column:product_id"`
	Color     string `gorm:"column:color"`
	Stock     int    `gorm:"column:stock"`
	SKU       string `gorm:"column:sku"`
}

func (aliasVariant) TableName() string { return "alias_variants" }

type aliasDetail struct {
	ID          string `gorm:"column:id;primaryKey"`
	Description string `gorm:"column:description"`
}

type aliasOther struct {
	ID string `gorm:"column:id;primaryKey"`
}

func (aliasOther) TableName() string { return "alias_other" }

func buildQueryWithAliases(t *testing.T, input string, model any, dest any, aliases Aliases) (*gorm.DB, string, []any) {
	t.Helper()
	db := newDryRunDB(t)

	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}

	db, err = WithAliases(db, model, aliases)
	if err != nil {
		t.Fatalf("WithAliases: %v", err)
	}

	out, err := BuildQuery(db, node, model)
	if err != nil {
		t.Fatalf("BuildQuery(%q): %v", input, err)
	}

	out.Find(dest)
	return out, out.Statement.SQL.String(), out.Statement.Vars
}

func TestWithAliasesBasicHasManyViaJunction(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "categories.name==Category A", aliasProduct{}, &aliasProduct{}, Aliases{
		"categories": "Cats.Category",
	})

	assertContains(t, sql, "LEFT JOIN alias_product_categories Cats ON Cats.product_id = alias_products.id")
	assertContains(t, sql, "LEFT JOIN alias_categories Cats__Category ON Cats__Category.category_id = Cats.id")
	assertContains(t, sql, "WHERE Cats__Category.name = ?")
	if len(vars) != 1 || vars[0] != "Category A" {
		t.Errorf("expected vars [Category A], got %v", vars)
	}
}

func TestWithAliasesHasOne(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "brand.name==Apple", aliasProduct{}, &aliasProduct{}, Aliases{
		"brand": "Brand",
	})

	assertContains(t, sql, "LEFT JOIN alias_brands Brand ON Brand.brand_id = alias_products.id")
	assertContains(t, sql, "WHERE Brand.name = ?")
	if len(vars) != 1 || vars[0] != "Apple" {
		t.Errorf("expected vars [Apple], got %v", vars)
	}
}

func TestWithAliasesDirectHasMany(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "variants.color==Space Gray", aliasProduct{}, &aliasProduct{}, Aliases{
		"variants": "Vars",
	})

	assertContains(t, sql, "LEFT JOIN alias_variants Vars ON Vars.product_id = alias_products.id")
	assertContains(t, sql, "WHERE Vars.color = ?")
	if len(vars) != 1 || vars[0] != "Space Gray" {
		t.Errorf("expected vars [Space Gray], got %v", vars)
	}
}

func TestWithAliasesMultiplePerRelation(t *testing.T) {
	tests := []string{
		"categories.name==A",
		"category.name==B",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, sql, vars := buildQueryWithAliases(t, input, aliasProduct{}, &aliasProduct{}, Aliases{
				"categories": "Cats.Category",
				"category":   "Cats.Category",
			})

			assertContains(t, sql, "LEFT JOIN alias_product_categories Cats ON Cats.product_id = alias_products.id")
			assertContains(t, sql, "LEFT JOIN alias_categories Cats__Category ON Cats__Category.category_id = Cats.id")
			assertContains(t, sql, "WHERE Cats__Category.name = ?")
			if len(vars) != 1 {
				t.Errorf("expected 1 var, got %v", vars)
			}
		})
	}
}

func TestWithAliasesLongestPrefixWins(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "categories.parent.name==X", aliasProduct{}, &aliasProduct{}, Aliases{
		"categories":        "Cats.Category",
		"categories.parent": "Cats.Category",
	})

	assertContains(t, sql, "WHERE Cats__Category.name = ?")
	if len(vars) != 1 || vars[0] != "X" {
		t.Errorf("expected vars [X], got %v", vars)
	}
}

func TestWithAliasesLeafRename(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "brandLabel==Apple", aliasProduct{}, &aliasProduct{}, Aliases{
		"brandLabel": "Brand.Name",
	})

	assertContains(t, sql, "LEFT JOIN alias_brands Brand ON Brand.brand_id = alias_products.id")
	assertContains(t, sql, "WHERE Brand.name = ?")
	if len(vars) != 1 || vars[0] != "Apple" {
		t.Errorf("expected vars [Apple], got %v", vars)
	}
}

func TestWithAliasesNegatedHasMany(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "variants.color!=Space Gray", aliasProduct{}, &aliasProduct{}, Aliases{
		"variants": "Vars",
	})

	assertContains(t, sql, "alias_products.id NOT IN (SELECT t0.product_id FROM alias_variants t0 WHERE t0.color = ?)")
	if len(vars) != 1 || vars[0] != "Space Gray" {
		t.Errorf("expected vars [Space Gray], got %v", vars)
	}
}

func TestWithAliasesNegatedHasManyWildcard(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "variants.color!=st*ff", aliasProduct{}, &aliasProduct{}, Aliases{
		"variants": "Vars",
	})

	assertContains(t, sql, "alias_products.id NOT IN (SELECT t0.product_id FROM alias_variants t0 WHERE t0.color ILIKE ? ESCAPE '\\')")
	if len(vars) != 1 || vars[0] != "st%ff" {
		t.Errorf("expected pattern st%%ff, got %v", vars)
	}
}

func TestWithAliasesNegatedHasManyOut(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "variants.color=out=(A,B)", aliasProduct{}, &aliasProduct{}, Aliases{
		"variants": "Vars",
	})

	assertContains(t, sql, "alias_products.id NOT IN (SELECT t0.product_id FROM alias_variants t0 WHERE t0.color IN (?,?))")
	if len(vars) != 2 || vars[0] != "A" || vars[1] != "B" {
		t.Errorf("expected vars [A B], got %v", vars)
	}
}

func TestWithAliasesNegatedJunctionNotInSubquery(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "categories.name!=A", aliasProduct{}, &aliasProduct{}, Aliases{
		"categories": "Cats.Category",
	})

	assertContains(t, sql, "LEFT JOIN alias_product_categories Cats ON Cats.product_id = alias_products.id")
	assertContains(t, sql, "LEFT JOIN alias_categories Cats__Category ON Cats__Category.category_id = Cats.id")
	assertContains(t, sql, "WHERE Cats__Category.name <> ?")
	if len(vars) != 1 || vars[0] != "A" {
		t.Errorf("expected vars [A], got %v", vars)
	}
}

func TestWithAliasesAndOr(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "categories.name==A;brand.name==B,name==Z", aliasProduct{}, &aliasProduct{}, Aliases{
		"categories": "Cats.Category",
		"brand":      "Brand",
	})

	assertContains(t, sql, "Cats__Category.name = ?")
	assertContains(t, sql, "Brand.name = ?")
	assertContains(t, sql, "alias_products.name = ?")
	if len(vars) != 3 {
		t.Errorf("expected 3 vars, got %v", vars)
	}
}

func TestWithAliasesSort(t *testing.T) {
	db := newDryRunDB(t)

	db, err := WithAliases(db, aliasProduct{}, Aliases{"categories": "Cats.Category"})
	if err != nil {
		t.Fatalf("WithAliases: %v", err)
	}

	sorts := []Sort{{Field: "categories.name", Desc: true}}
	out, err := ApplySort(db, sorts, aliasProduct{})
	if err != nil {
		t.Fatalf("ApplySort: %v", err)
	}

	out.Find(&aliasProduct{})
	sql := out.Statement.SQL.String()

	assertContains(t, sql, "LEFT JOIN alias_product_categories Cats ON Cats.product_id = alias_products.id")
	assertContains(t, sql, "LEFT JOIN alias_categories Cats__Category ON Cats__Category.category_id = Cats.id")
	assertContains(t, sql, "ORDER BY Cats__Category.name DESC")
}

func TestWithAliasesSortNoAlias(t *testing.T) {
	db := newDryRunDB(t)

	db, err := WithAliases(db, aliasProduct{}, Aliases{"categories": "Cats.Category"})
	if err != nil {
		t.Fatalf("WithAliases: %v", err)
	}

	sorts := []Sort{{Field: "name", Desc: true}}
	out, err := ApplySort(db, sorts, aliasProduct{})
	if err != nil {
		t.Fatalf("ApplySort: %v", err)
	}

	out.Find(&aliasProduct{})
	sql := out.Statement.SQL.String()

	assertContains(t, sql, "ORDER BY alias_products.name DESC")
}

func TestWithAliasesUnmatchedSelectorUnaffected(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "name==John", aliasProduct{}, &aliasProduct{}, Aliases{
		"categories": "Cats.Category",
	})

	assertContains(t, sql, "WHERE alias_products.name = ?")
	if len(vars) != 1 || vars[0] != "John" {
		t.Errorf("expected vars [John], got %v", vars)
	}
}

func TestWithAliasesInternalPathStillWorks(t *testing.T) {
	_, sql, vars := buildQueryWithAliases(t, "Cats.Category.name==A", aliasProduct{}, &aliasProduct{}, Aliases{
		"categories": "Cats.Category",
	})

	assertContains(t, sql, "LEFT JOIN alias_product_categories Cats ON Cats.product_id = alias_products.id")
	assertContains(t, sql, "LEFT JOIN alias_categories Cats__Category ON Cats__Category.category_id = Cats.id")
	assertContains(t, sql, "WHERE Cats__Category.name = ?")
	if len(vars) != 1 || vars[0] != "A" {
		t.Errorf("expected vars [A], got %v", vars)
	}
}

func TestWithAliasesNoAliasRegistered(t *testing.T) {
	db := newDryRunDB(t)
	node, err := Parse("categories.name==A")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	_, err = BuildQuery(db, node, aliasProduct{})
	if err == nil {
		t.Fatal("expected error for unknown alias without registration")
	}
}

func TestWithAliasesInvalidValue(t *testing.T) {
	db := newDryRunDB(t)
	_, err := WithAliases(db, aliasProduct{}, Aliases{
		"categories": "Categoryz",
	})
	if err == nil {
		t.Fatal("expected error for invalid alias value path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %v", err)
	}
}

func TestWithAliasesInvalidModel(t *testing.T) {
	db := newDryRunDB(t)
	_, err := WithAliases(db, "not-a-struct", Aliases{"x": "Y"})
	if err == nil {
		t.Fatal("expected error for non-struct model")
	}
	if !strings.Contains(err.Error(), "must be a struct") {
		t.Errorf("expected 'must be a struct', got %v", err)
	}
}

func TestWithAliasesEmptyKey(t *testing.T) {
	db := newDryRunDB(t)
	_, err := WithAliases(db, aliasProduct{}, Aliases{
		"": "Brand",
	})
	if err == nil {
		t.Fatal("expected error for empty alias key")
	}
}

func TestWithAliasesEmptyValue(t *testing.T) {
	db := newDryRunDB(t)
	_, err := WithAliases(db, aliasProduct{}, Aliases{
		"categories": "",
	})
	if err == nil {
		t.Fatal("expected error for empty alias value")
	}
}

func TestWithAliasesASTNotMutated(t *testing.T) {
	db := newDryRunDB(t)
	originalSelector := "categories.name"
	node, err := Parse(originalSelector + "==A")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	db, err = WithAliases(db, aliasProduct{}, Aliases{"categories": "Cats.Category"})
	if err != nil {
		t.Fatalf("WithAliases: %v", err)
	}

	_, err = BuildQuery(db, node, aliasProduct{})
	if err != nil {
		t.Fatalf("BuildQuery: %v", err)
	}

	cmp, ok := node.(*ComparisonNode)
	if !ok {
		t.Fatalf("expected ComparisonNode")
	}
	if cmp.Selector != originalSelector {
		t.Errorf("original selector mutated: got %q, want %q", cmp.Selector, originalSelector)
	}
}

func TestWithAliasesBuildQueryWithParams(t *testing.T) {
	db := newDryRunDB(t)

	db, err := WithAliases(db, aliasProduct{}, Aliases{"categories": "Cats.Category"})
	if err != nil {
		t.Fatalf("WithAliases: %v", err)
	}

	p := &Params{
		Pagination: Pagination{Page: 1, Limit: 10},
		Filter:     &ComparisonNode{Selector: "categories.name", Operator: "==", Arguments: "A"},
		Sorts:      []Sort{{Field: "categories.name", Desc: true}},
	}

	out, err := BuildQueryWithParams(db, p, aliasProduct{})
	if err != nil {
		t.Fatalf("BuildQueryWithParams: %v", err)
	}

	out.Find(&aliasProduct{})
	sql := out.Statement.SQL.String()

	assertContains(t, sql, "LEFT JOIN alias_product_categories Cats ON Cats.product_id = alias_products.id")
	assertContains(t, sql, "LEFT JOIN alias_categories Cats__Category ON Cats__Category.category_id = Cats.id")
	assertContains(t, sql, "WHERE Cats__Category.name = ?")
	assertContains(t, sql, "ORDER BY Cats__Category.name DESC")
}

func TestWithAliasesBuildPageableQuery(t *testing.T) {
	db := newDryRunDB(t)

	db, err := WithAliases(db, aliasProduct{}, Aliases{"categories": "Cats.Category"})
	if err != nil {
		t.Fatalf("WithAliases: %v", err)
	}

	p := &Params{
		Pagination: Pagination{Page: 2, Limit: 10},
		Filter:     &ComparisonNode{Selector: "categories.name", Operator: "==", Arguments: "A"},
		Sorts:      []Sort{{Field: "categories.name", Desc: true}},
	}

	pq, err := BuildPageableQuery(db, p, aliasProduct{})
	if err != nil {
		t.Fatalf("BuildPageableQuery: %v", err)
	}
	if pq.Page != 2 || pq.Limit != 10 || pq.Offset != 10 {
		t.Errorf("sanitized pagination wrong: %+v", pq)
	}

	q := pq.NewQuery()
	q.Find(&aliasProduct{})
	sql := q.Statement.SQL.String()

	assertContains(t, sql, "LEFT JOIN alias_product_categories Cats ON Cats.product_id = alias_products.id")
	assertContains(t, sql, "LEFT JOIN alias_categories Cats__Category ON Cats__Category.category_id = Cats.id")
	assertContains(t, sql, "WHERE Cats__Category.name = ?")
	assertContains(t, sql, "ORDER BY Cats__Category.name DESC")
}

func TestWithAliasesSeparateDBInstances(t *testing.T) {
	db := newDryRunDB(t)

	dbProduct, err := WithAliases(db, aliasProduct{}, Aliases{"categories": "Cats.Category"})
	if err != nil {
		t.Fatalf("WithAliases Product: %v", err)
	}

	dbOther, err := WithAliases(db, aliasOther{}, Aliases{"x": "ID"})
	if err != nil {
		t.Fatalf("WithAliases other: %v", err)
	}

	node, err := Parse("categories.name==A")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	_, err = BuildQuery(dbProduct, node, aliasProduct{})
	if err != nil {
		t.Fatalf("product query should work: %v", err)
	}

	_, err = BuildQuery(dbOther, node, aliasProduct{})
	if err == nil {
		t.Fatal("query on wrong db should not find alias")
	}
}

func TestResolveAliases(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		aliases  Aliases
		want     string
		wantErr  bool
	}{
		{"no aliases", "a.b", nil, "a.b", false},
		{"no match", "a.b", Aliases{"x": "Y"}, "a.b", false},
		{"exact match", "categories", Aliases{"categories": "Cats.Category"}, "Cats.Category", false},
		{"prefix match", "categories.name", Aliases{"categories": "Cats.Category"}, "Cats.Category.name", false},
		{"longest prefix", "categories.parent.name", Aliases{"categories": "Cats", "categories.parent": "Cats.Parent"}, "Cats.Parent.name", false},
		{"empty key", "a.b", Aliases{"": "X"}, "", true},
		{"empty value", "a.b", Aliases{"a": ""}, "", true},
		{"single segment no match", "name", Aliases{"categories": "Cats"}, "name", false},
		{"alias key with dots", "cat.name", Aliases{"cat": "Cats.Category", "cat.name": "Cats.Category.Name"}, "Cats.Category.Name", false},
		{"alias leaf rename exact", "categoryLabel", Aliases{"categoryLabel": "Brand.Name"}, "Brand.Name", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAliases(tt.selector, tt.aliases)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveAliasesAmbiguousPrefix(t *testing.T) {
	got, _ := resolveAliases("a.b.c", Aliases{"a": "X", "a.b": "Y.Z"})
	if got != "Y.Z.c" {
		t.Errorf("longest prefix: got %q, want %q", got, "Y.Z.c")
	}
}

func TestResolveAliasesExactMatchVersusPrefix(t *testing.T) {
	got, _ := resolveAliases("a", Aliases{"a": "X", "a.b": "Y"})
	if got != "X" {
		t.Errorf("exact match: got %q, want %q", got, "X")
	}
}
