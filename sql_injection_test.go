package rsql

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These tests execute real SQL against an in-memory SQLite database to
// demonstrate that values stay parameterized (no string break-out) while the
// SQL that RSQL generates can still produce surprising results through
// operator semantics: empty IN lists, three-valued NULL logic, and ILIKE
// patterns. None of these payloads are errors: they slip through RSQL
// because the syntax is valid, which is exactly why they need documenting.

type injUser struct {
	ID     uint     `gorm:"column:id;primaryKey"`
	Name   string   `gorm:"column:name"`
	Status *string  `gorm:"column:status"`
}

func (injUser) TableName() string { return "inj_users" }

func newInjectionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Migrator().DropTable(&injUser{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := db.AutoMigrate(&injUser{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedInjUsers inserts six rows: four ACTIVE, one PENDING, one DELETED, and
// one row whose status is NULL. It returns the total row count.
func seedInjUsers(t *testing.T, db *gorm.DB) int {
	t.Helper()
	active := "ACTIVE"
	pending := "PENDING"
	deleted := "DELETED"
	rows := []injUser{
		{Name: "admin", Status: &active},
		{Name: "Admin", Status: &active},
		{Name: "ADMIN", Status: &pending},
		{Name: "", Status: &active},
		{Name: "ghost", Status: &deleted},
		{Name: "nullstatus"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return len(rows)
}

func runQuery(t *testing.T, db *gorm.DB, input string) []injUser {
	t.Helper()
	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	out, err := BuildQuery(db, node, injUser{})
	if err != nil {
		t.Fatalf("BuildQuery(%q): %v", input, err)
	}
	var users []injUser
	if err := out.Find(&users).Error; err != nil {
		t.Fatalf("Find(%q): %v", input, err)
	}
	return users
}

// TestSQLInjectionPayloadsStayParameterized proves the classic payloads cannot
// break out of the SQL string: the payload becomes the bind argument, so the
// equality matches nothing instead of returning every row.
func TestSQLInjectionPayloadsStayParameterized(t *testing.T) {
	db := newInjectionDB(t)
	seedInjUsers(t, db)

	for _, input := range []string{
		"name==' OR 1=1--",
		"name==' OR '1'='1",
	} {
		t.Run(input, func(t *testing.T) {
			if users := runQuery(t, db, input); len(users) != 0 {
				t.Errorf("payload %q should match 0 rows (literal), got %d rows", input, len(users))
			}
		})
	}
}

// TestSQLInjectionEmptyOutListReturnsAllRows documents a logic bypass: a
// NOT IN over an empty list is true for every row, so =out=() acts as a
// filter reset that returns the whole table.
func TestSQLInjectionEmptyOutListReturnsAllRows(t *testing.T) {
	db := newInjectionDB(t)
	total := seedInjUsers(t, db)

	users := runQuery(t, db, "status=out=()")
	if len(users) != total {
		t.Errorf("=out=() should return all %d rows, got %d", total, len(users))
	}
}

// TestSQLInjectionEmptyInListReturnsNone documents the mirror case: IN over
// an empty list is false for every row, so =in=() silently matches nothing.
func TestSQLInjectionEmptyInListReturnsNone(t *testing.T) {
	db := newInjectionDB(t)
	seedInjUsers(t, db)

	if users := runQuery(t, db, "status=in=()"); len(users) != 0 {
		t.Errorf("=in=() should return 0 rows, got %d", len(users))
	}
}

// TestSQLInjectionOrTautologyExcludesNulls documents three-valued NULL logic:
// `status==ACTIVE,status!=ACTIVE` covers every non-null value, yet the row
// with a NULL status is silently dropped because NULL comparisons are unknown.
func TestSQLInjectionOrTautologyExcludesNulls(t *testing.T) {
	db := newInjectionDB(t)
	seedInjUsers(t, db)

	users := runQuery(t, db, "status==ACTIVE,status!=ACTIVE")
	if len(users) != 5 {
		t.Errorf("tautology should return 5 non-null rows (NULL excluded), got %d", len(users))
	}
}

// TestSQLInjectionNotEqualExcludesNulls documents that `!=` does not include
// NULL rows: "everything not ACTIVE" silently misses null statuses.
func TestSQLInjectionNotEqualExcludesNulls(t *testing.T) {
	db := newInjectionDB(t)
	seedInjUsers(t, db)

	users := runQuery(t, db, "status!=ACTIVE")
	if len(users) != 2 {
		t.Errorf("status!=ACTIVE should return 2 non-null rows (NULL excluded), got %d", len(users))
	}
}

// TestSQLInjectionNegatedWildcardSQL documents that `name!=*` becomes
// `NOT ILIKE '%'`, which excludes every non-null value; in practice it is a
// back-door for selecting rows whose column is NULL.
func TestSQLInjectionNegatedWildcardSQL(t *testing.T) {
	_, sql, vars := buildQuery(t, "name!=*", qbUser{}, &qbUser{})

	assertContains(t, sql, "qb_users.name NOT ILIKE ? ESCAPE '\\'")
	if len(vars) != 1 || vars[0] != "%" {
		t.Errorf("expected pattern %%, got %v", vars)
	}
}

// TestSQLInjectionWildcardMatchesAllSQL documents that `name==*` becomes
// `ILIKE '%'`, which matches every non-null value; a single character filter
// that silently disables column-level restrictions.
func TestSQLInjectionWildcardMatchesAllSQL(t *testing.T) {
	_, sql, vars := buildQuery(t, "name==*", qbUser{}, &qbUser{})

	assertContains(t, sql, "qb_users.name ILIKE ? ESCAPE '\\'")
	if len(vars) != 1 || vars[0] != "%" {
		t.Errorf("expected pattern %%, got %v", vars)
	}
}
