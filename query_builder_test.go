package rsql

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type qbUser struct {
	ID        string       `gorm:"column:id;primaryKey"`
	Name      string       `gorm:"column:name"`
	Email     string       `gorm:"column:email"`
	CreatedAt string       `gorm:"column:created_at"`
	Roles     []qbUserRole `gorm:"foreignKey:UserID;references:ID"`
}

func (qbUser) TableName() string { return "qb_users" }

type qbUserRole struct {
	ID       string `gorm:"column:id;primaryKey"`
	UserID   string `gorm:"column:user_id"`
	RoleID   string `gorm:"column:role_id"`
	RoleName string `gorm:"column:role_name"`
	Role     qbRole `gorm:"foreignKey:RoleID;references:ID"`
}

func (qbUserRole) TableName() string { return "qb_user_roles" }

type qbRole struct {
	ID   string `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

func (qbRole) TableName() string { return "qb_roles" }

type qbLevel1 struct {
	ID  string   `gorm:"column:id;primaryKey"`
	AID string   `gorm:"column:a_id"`
	A   qbLevel2 `gorm:"foreignKey:AID;references:ID"`
}

func (qbLevel1) TableName() string { return "qb_level1" }

type qbLevel2 struct {
	ID  string   `gorm:"column:id;primaryKey"`
	BID string   `gorm:"column:b_id"`
	B   qbLevel3 `gorm:"foreignKey:BID;references:ID"`
}

func (qbLevel2) TableName() string { return "qb_level2" }

type qbLevel3 struct {
	ID  string   `gorm:"column:id;primaryKey"`
	CID string   `gorm:"column:c_id"`
	C   qbLevel4 `gorm:"foreignKey:CID;references:ID"`
}

func (qbLevel3) TableName() string { return "qb_level3" }

type qbLevel4 struct {
	ID  string   `gorm:"column:id;primaryKey"`
	DID string   `gorm:"column:d_id"`
	D   qbLevel5 `gorm:"foreignKey:DID;references:ID"`
}

func (qbLevel4) TableName() string { return "qb_level4" }

type qbLevel5 struct {
	ID string `gorm:"column:id;primaryKey"`
}

func (qbLevel5) TableName() string { return "qb_level5" }

func newDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DryRun: true,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open gorm in dry-run mode: %v", err)
	}
	return db
}

func buildQuery(t *testing.T, input string, model any, dest any) (*gorm.DB, string, []any) {
	t.Helper()
	db := newDryRunDB(t)

	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}

	out, err := BuildQuery(db, node, model)
	if err != nil {
		t.Fatalf("BuildQuery(%q): %v", input, err)
	}

	out.Find(dest)
	return out, out.Statement.SQL.String(), out.Statement.Vars
}

func assertContains(t *testing.T, sql, want string) {
	t.Helper()
	if !strings.Contains(sql, want) {
		t.Errorf("SQL does not contain %q\nSQL: %s", want, sql)
	}
}

func TestBuildQueryNilNode(t *testing.T) {
	db := newDryRunDB(t)
	out, err := BuildQuery(db, nil, qbUser{})
	if err != nil {
		t.Fatalf("nil node should not error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil *gorm.DB")
	}
}

func TestBuildQueryRootModelMustBeStruct(t *testing.T) {
	db := newDryRunDB(t)
	node := &ComparisonNode{Selector: "name", Operator: "==", Arguments: "John"}
	_, err := BuildQuery(db, node, "not-a-struct")
	if err == nil {
		t.Fatal("expected error for non-struct root model")
	}
}

func TestBuildQuerySimpleEquality(t *testing.T) {
	_, sql, vars := buildQuery(t, "name==John", qbUser{}, &qbUser{})

	assertContains(t, sql, "WHERE qb_users.name = ?")
	if len(vars) != 1 || vars[0] != "John" {
		t.Errorf("expected vars [John], got %v", vars)
	}
}

func TestBuildQueryOperators(t *testing.T) {
	tests := []struct {
		input      string
		wantClause string
		wantVar    any
	}{
		{"price>100", "qb_users.price > ?", "100"},
		{"price>=100", "qb_users.price >= ?", "100"},
		{"price<100", "qb_users.price < ?", "100"},
		{"price<=100", "qb_users.price <= ?", "100"},
		{"status!=ACTIVE", "qb_users.status <> ?", "ACTIVE"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, sql, vars := buildQuery(t, tt.input, qbUser{}, &qbUser{})

			assertContains(t, sql, "WHERE "+tt.wantClause)
			if len(vars) != 1 || vars[0] != tt.wantVar {
				t.Errorf("expected vars [%v], got %v", tt.wantVar, vars)
			}
		})
	}
}

func TestBuildQueryWildcard(t *testing.T) {
	tests := []struct {
		input       string
		wantPattern string
	}{
		{"name==Jo*", "Jo%"},
		{"name==*n", "%n"},
		{"name==*ohn*", "%ohn%"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, sql, vars := buildQuery(t, tt.input, qbUser{}, &qbUser{})

			assertContains(t, sql, "WHERE qb_users.name ILIKE ?")
			if len(vars) != 1 || vars[0] != tt.wantPattern {
				t.Errorf("expected pattern %q, got %v", tt.wantPattern, vars)
			}
		})
	}
}

func TestBuildQueryEqualityNull(t *testing.T) {
	for _, v := range []string{"null", "NULL", "Null"} {
		t.Run(v, func(t *testing.T) {
			_, sql, vars := buildQuery(t, "name=="+v, qbUser{}, &qbUser{})

			assertContains(t, sql, "WHERE qb_users.name IS NULL")
			if len(vars) != 0 {
				t.Errorf("expected no vars, got %v", vars)
			}
		})
	}
}

func TestBuildQueryNotEqualNull(t *testing.T) {
	_, sql, vars := buildQuery(t, "name!=null", qbUser{}, &qbUser{})

	assertContains(t, sql, "WHERE qb_users.name IS NOT NULL")
	if len(vars) != 0 {
		t.Errorf("expected no vars, got %v", vars)
	}
}

func TestBuildQueryNullWildcardStillSearch(t *testing.T) {
	tests := []struct {
		input       string
		wantPattern string
	}{
		{"name==*null*", "%null%"},
		{"name==null*", "null%"},
		{"name==*null", "%null"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, sql, vars := buildQuery(t, tt.input, qbUser{}, &qbUser{})

			assertContains(t, sql, "WHERE qb_users.name ILIKE ?")
			if len(vars) != 1 || vars[0] != tt.wantPattern {
				t.Errorf("expected pattern %q, got %v", tt.wantPattern, vars)
			}
		})
	}
}

func TestBuildQueryNullLiteralUnaffected(t *testing.T) {
	_, sql, vars := buildQuery(t, "name==nullvalue", qbUser{}, &qbUser{})

	assertContains(t, sql, "WHERE qb_users.name = ?")
	if len(vars) != 1 || vars[0] != "nullvalue" {
		t.Errorf("expected vars [nullvalue], got %v", vars)
	}
}

func TestBuildQueryNullTrailingSpace(t *testing.T) {
	_, sql, vars := buildQuery(t, "name==null ", qbUser{}, &qbUser{})

	assertContains(t, sql, "WHERE qb_users.name IS NULL")
	if len(vars) != 0 {
		t.Errorf("expected no vars, got %v", vars)
	}
}

func TestBuildQueryNullCombined(t *testing.T) {
	_, sql, vars := buildQuery(t, "name==null;status==ACTIVE", qbUser{}, &qbUser{})

	assertContains(t, sql, "qb_users.name IS NULL AND qb_users.status = ?")
	if len(vars) != 1 || vars[0] != "ACTIVE" {
		t.Errorf("expected vars [ACTIVE], got %v", vars)
	}
}

func TestBuildQueryInOut(t *testing.T) {
	_, sql, vars := buildQuery(t, "status=in=(ACTIVE,PENDING)", qbUser{}, &qbUser{})
	assertContains(t, sql, "WHERE qb_users.status IN (?,?)")
	if len(vars) != 2 || vars[0] != "ACTIVE" || vars[1] != "PENDING" {
		t.Errorf("expected vars [ACTIVE PENDING], got %v", vars)
	}

	_, sql, vars = buildQuery(t, "status=out=(DELETED,ARCHIVED)", qbUser{}, &qbUser{})
	assertContains(t, sql, "WHERE qb_users.status NOT IN (?,?)")
	if len(vars) != 2 || vars[0] != "DELETED" || vars[1] != "ARCHIVED" {
		t.Errorf("expected vars [DELETED ARCHIVED], got %v", vars)
	}
}

func TestBuildQueryInNullIsLiteral(t *testing.T) {
	_, sql, vars := buildQuery(t, "status=in=(ACTIVE,null)", qbUser{}, &qbUser{})

	assertContains(t, sql, "WHERE qb_users.status IN (?,?)")
	if len(vars) != 2 || vars[0] != "ACTIVE" || vars[1] != "null" {
		t.Errorf("expected vars [ACTIVE null], got %v", vars)
	}
}

func TestBuildQueryOutNullIsLiteral(t *testing.T) {
	_, sql, vars := buildQuery(t, "status=out=(DELETED,null)", qbUser{}, &qbUser{})

	assertContains(t, sql, "WHERE qb_users.status NOT IN (?,?)")
	if len(vars) != 2 || vars[0] != "DELETED" || vars[1] != "null" {
		t.Errorf("expected vars [DELETED null], got %v", vars)
	}
}

func TestBuildQueryJoinHasMany(t *testing.T) {
	_, sql, vars := buildQuery(t, "roles.RoleName==staff", qbUser{}, &qbUser{})

	assertContains(t, sql, "LEFT JOIN qb_user_roles Roles ON Roles.user_id = qb_users.id")
	assertContains(t, sql, "WHERE Roles.role_name = ?")
	if len(vars) != 1 || vars[0] != "staff" {
		t.Errorf("expected vars [staff], got %v", vars)
	}
}

func TestBuildQueryJoinNested(t *testing.T) {
	_, sql, vars := buildQuery(t, "roles.Role.name==admin", qbUser{}, &qbUser{})

	assertContains(t, sql, "LEFT JOIN qb_user_roles Roles ON Roles.user_id = qb_users.id")
	assertContains(t, sql, "LEFT JOIN qb_roles Roles__Role ON Roles__Role.role_id = Roles.id")
	assertContains(t, sql, "WHERE Roles__Role.name = ?")
	if len(vars) != 1 || vars[0] != "admin" {
		t.Errorf("expected vars [admin], got %v", vars)
	}
}

func TestBuildQueryNegatedHasMany(t *testing.T) {
	_, sql, vars := buildQuery(t, "roles.RoleName!=staff", qbUser{}, &qbUser{})

	assertContains(t, sql, "qb_users.id NOT IN (SELECT t0.user_id FROM qb_user_roles t0 WHERE t0.role_name = ?)")
	if len(vars) != 1 || vars[0] != "staff" {
		t.Errorf("expected vars [staff], got %v", vars)
	}
}

func TestBuildQueryNegatedHasManyIn(t *testing.T) {
	_, sql, vars := buildQuery(t, "roles.RoleName=out=(staff,admin)", qbUser{}, &qbUser{})

	assertContains(t, sql, "qb_users.id NOT IN (SELECT t0.user_id FROM qb_user_roles t0 WHERE t0.role_name IN (?,?))")
	if len(vars) != 2 || vars[0] != "staff" || vars[1] != "admin" {
		t.Errorf("expected vars [staff admin], got %v", vars)
	}
}

func TestBuildQueryAndOr(t *testing.T) {
	_, sql, vars := buildQuery(t, "name==John;status==ACTIVE,status==PENDING", qbUser{}, &qbUser{})

	assertContains(t, sql, "(qb_users.name = ? AND qb_users.status = ?) OR qb_users.status = ?")
	if len(vars) != 3 {
		t.Errorf("expected 3 vars, got %v", vars)
	}
}

func TestBuildQueryDeepJoin(t *testing.T) {
	_, sql, _ := buildQuery(t, "A.B.C.D.id==x", qbLevel1{}, &qbLevel1{})

	assertContains(t, sql, "LEFT JOIN qb_level2 A ON A.a_id = qb_level1.id")
	assertContains(t, sql, "LEFT JOIN qb_level3 A__B ON A__B.b_id = A.id")
	assertContains(t, sql, "LEFT JOIN qb_level4 A__B__C ON A__B__C.c_id = A__B.id")
	assertContains(t, sql, "LEFT JOIN qb_level5 A__B__C__D ON A__B__C__D.d_id = A__B__C.id")
}

func TestBuildQueryInvalidSelector(t *testing.T) {
	db := newDryRunDB(t)
	node, err := Parse("unknown.name==x")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = BuildQuery(db, node, qbUser{})
	if err == nil {
		t.Fatal("expected error for unknown relation")
	}
}

func TestBuildQueryJoinDepthExceeded(t *testing.T) {
	db := newDryRunDB(t)
	node, err := Parse("A.B.C.D.X.Y.Z==x")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, err = BuildQuery(db, node, qbLevel1{})
	if err == nil {
		t.Fatal("expected depth error")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected depth error message, got %v", err)
	}
}
