package rsql

import (
	"reflect"
	"testing"
)

type testRelation struct {
	ID   string `gorm:"column:id;primaryKey"`
	Name string `gorm:"column:name"`
}

func (testRelation) TableName() string { return "auth_roles" }

type testJunction struct {
	ID     string       `gorm:"column:id;primaryKey"`
	UserID string       `gorm:"column:user_id"`
	RoleID string       `gorm:"column:role_id"`
	Role   testRelation `gorm:"foreignKey:RoleID;references:ID"`
}

func (testJunction) TableName() string { return "auth_user_roles" }

type testUser struct {
	ID    string         `gorm:"column:id;primaryKey"`
	Name  string         `gorm:"column:name"`
	Roles []testJunction `gorm:"foreignKey:UserID;references:ID"`
}

func (testUser) TableName() string { return "mst_users" }

type testDeep1 struct {
	ID string `gorm:"column:id;primaryKey"`
	B  testDeep2
}

type testDeep2 struct {
	ID string `gorm:"column:id;primaryKey"`
	C  testDeep3
}

type testDeep3 struct {
	ID string `gorm:"column:id;primaryKey"`
	D  testDeep4
}

type testDeep4 struct {
	ID string `gorm:"column:id;primaryKey"`
}

func TestValidatorSimpleSelector(t *testing.T) {
	v := &Validator{rootModel: reflect.TypeOf(testUser{}), maxJoinDepth: defaultMaxJoinDepth}

	node := &ComparisonNode{Selector: "status", Operator: "==", Arguments: "ACTIVE"}
	if err := v.Validate(node); err != nil {
		t.Fatalf("simple selector should be valid: %v", err)
	}
}

func TestValidatorNilNode(t *testing.T) {
	v := &Validator{rootModel: reflect.TypeOf(testUser{}), maxJoinDepth: defaultMaxJoinDepth}

	if err := v.Validate(nil); err != nil {
		t.Fatalf("nil node should be valid: %v", err)
	}
}

func TestValidatorRegisteredRelation(t *testing.T) {
	v := &Validator{rootModel: reflect.TypeOf(testUser{}), maxJoinDepth: defaultMaxJoinDepth}

	tests := []string{
		"roles.Name",
		"roles.Role.Name",
		"Roles.name",
	}

	for _, selector := range tests {
		t.Run(selector, func(t *testing.T) {
			node := &ComparisonNode{Selector: selector, Operator: "==", Arguments: "test"}
			if err := v.Validate(node); err != nil {
				t.Errorf("expected valid selector %q, got error: %v", selector, err)
			}
		})
	}
}

func TestValidatorUnregisteredRelation(t *testing.T) {
	v := &Validator{rootModel: reflect.TypeOf(testUser{}), maxJoinDepth: defaultMaxJoinDepth}

	node := &ComparisonNode{Selector: "manufacturer.name", Operator: "==", Arguments: "test"}
	err := v.Validate(node)
	if err == nil {
		t.Fatal("expected error for unregistered relation 'manufacturer'")
	}
}

func TestValidatorMaxJoinDepth(t *testing.T) {
	v := &Validator{rootModel: reflect.TypeOf(testDeep1{}), maxJoinDepth: 2}

	valid := "B.C.Name"
	invalid := "B.C.D.Name"

	node := &ComparisonNode{Selector: valid, Operator: "==", Arguments: "test"}
	if err := v.Validate(node); err != nil {
		t.Errorf("selector %q (depth=2) should be valid with maxDepth=2: %v", valid, err)
	}

	node = &ComparisonNode{Selector: invalid, Operator: "==", Arguments: "test"}
	if err := v.Validate(node); err == nil {
		t.Errorf("selector %q (depth=3) should exceed maxDepth=2", invalid)
	}
}

func TestValidatorAndOrNodes(t *testing.T) {
	v := &Validator{rootModel: reflect.TypeOf(testUser{}), maxJoinDepth: defaultMaxJoinDepth}

	and := &AndNode{
		Children: []Node{
			&ComparisonNode{Selector: "name", Operator: "==", Arguments: "ACTIVE"},
			&ComparisonNode{Selector: "roles.name", Operator: "==", Arguments: "admin"},
		},
	}
	if err := v.Validate(and); err != nil {
		t.Fatalf("valid AND node should pass: %v", err)
	}

	or := &OrNode{
		Children: []Node{
			&ComparisonNode{Selector: "name", Operator: "==", Arguments: "ACTIVE"},
			&ComparisonNode{Selector: "invalid.name", Operator: "==", Arguments: "x"},
		},
	}
	if err := v.Validate(or); err == nil {
		t.Fatal("OR node with unregistered relation should fail")
	}
}
