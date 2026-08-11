package rsql

import (
	"strings"
	"testing"
)

func TestParseEmpty(t *testing.T) {
	node, err := Parse("")
	if err != nil {
		t.Fatalf("empty string should parse to nil, got error: %v", err)
	}
	if node != nil {
		t.Fatalf("empty string should return nil node, got %T", node)
	}
}

func TestParseSingleComparison(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		selector string
		operator string
		args     any
	}{
		{"eq", "status==ACTIVE", "status", "==", "ACTIVE"},
		{"ne", "status!=ACTIVE", "status", "!=", "ACTIVE"},
		{"gt", "price>100000", "price", ">", "100000"},
		{"gte", "price>=100000", "price", ">=", "100000"},
		{"lt", "price<100000", "price", "<", "100000"},
		{"lte", "price<=100000", "price", "<=", "100000"},
		{"dots_in_value", "status==ACTIVE.PENDING", "status", "==", "ACTIVE.PENDING"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			cmp, ok := node.(*ComparisonNode)
			if !ok {
				t.Fatalf("expected ComparisonNode, got %T", node)
			}
			if cmp.Selector != tt.selector {
				t.Errorf("selector: want %q, got %q", tt.selector, cmp.Selector)
			}
			if cmp.Operator != tt.operator {
				t.Errorf("operator: want %q, got %q", tt.operator, cmp.Operator)
			}
			if cmp.Arguments != tt.args {
				t.Errorf("arguments: want %q, got %q", tt.args, cmp.Arguments)
			}
		})
	}
}

func TestParseInOut(t *testing.T) {
	tests := []struct {
		name  string
		input string
		op    string
		args  any
	}{
		{"in", "status=in=(ACTIVE,PENDING)", "=in=", []string{"ACTIVE", "PENDING"}},
		{"out", "status=out=(DELETED,ARCHIVED)", "=out=", []string{"DELETED", "ARCHIVED"}},
		{"in_single", "status=in=(ACTIVE)", "=in=", []string{"ACTIVE"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			cmp := node.(*ComparisonNode)
			if cmp.Operator != tt.op {
				t.Errorf("operator: want %q, got %q", tt.op, cmp.Operator)
			}
			got := cmp.Arguments.([]string)
			if len(got) != len(tt.args.([]string)) {
				t.Fatalf("args length: want %d, got %d", len(tt.args.([]string)), len(got))
			}
			for i, v := range tt.args.([]string) {
				if got[i] != v {
					t.Errorf("args[%d]: want %q, got %q", i, v, got[i])
				}
			}
		})
	}
}

func TestParseNestedSelector(t *testing.T) {
	tests := []string{
		"category.name==Laptop",
		"user.role.name==ADMIN",
		"roles.role.name==operator",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			node, err := Parse(input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			cmp := node.(*ComparisonNode)
			if !containsDot(cmp.Selector) {
				t.Errorf("expected nested selector with '.', got %q", cmp.Selector)
			}
		})
	}
}

func TestParseAnd(t *testing.T) {
	node, err := Parse("status==ACTIVE;price>=100000")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	and, ok := node.(*AndNode)
	if !ok {
		t.Fatalf("expected AndNode, got %T", node)
	}
	if len(and.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(and.Children))
	}
}

func TestParseOr(t *testing.T) {
	node, err := Parse("status==ACTIVE,status==PENDING")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	or, ok := node.(*OrNode)
	if !ok {
		t.Fatalf("expected OrNode, got %T", node)
	}
	if len(or.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(or.Children))
	}
}

func TestParseGrouping(t *testing.T) {
	// status==ACTIVE;(price>100000,price<50000)
	node, err := Parse("status==ACTIVE;(price>100000,price<50000)")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	and, ok := node.(*AndNode)
	if !ok {
		t.Fatalf("expected AndNode, got %T", node)
	}
	if len(and.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(and.Children))
	}

	cmp, ok := and.Children[0].(*ComparisonNode)
	if !ok {
		t.Fatalf("expected first child to be ComparisonNode, got %T", and.Children[0])
	}
	if cmp.Selector != "status" {
		t.Errorf("expected selector 'status', got %q", cmp.Selector)
	}

	or, ok := and.Children[1].(*OrNode)
	if !ok {
		t.Fatalf("expected second child to be OrNode, got %T", and.Children[1])
	}
	if len(or.Children) != 2 {
		t.Fatalf("expected 2 children in OR group, got %d", len(or.Children))
	}
}

func TestParseNestedGroup(t *testing.T) {
	// (status==ACTIVE,status==PENDING);stock>0
	node, err := Parse("(status==ACTIVE,status==PENDING);stock>0")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	and, ok := node.(*AndNode)
	if !ok {
		t.Fatalf("expected AndNode, got %T", node)
	}
	if len(and.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(and.Children))
	}

	or, ok := and.Children[0].(*OrNode)
	if !ok {
		t.Fatalf("expected first child to be OrNode, got %T", and.Children[0])
	}
	if len(or.Children) != 2 {
		t.Fatalf("expected 2 children in OR group, got %d", len(or.Children))
	}

	cmp, ok := and.Children[1].(*ComparisonNode)
	if !ok {
		t.Fatalf("expected second child to be ComparisonNode, got %T", and.Children[1])
	}
	if cmp.Selector != "stock" {
		t.Errorf("expected selector 'stock', got %q", cmp.Selector)
	}
}

func TestParsePrecedence(t *testing.T) {
	// status==ACTIVE;price>100000,stock>0
	// Should be parsed as (status==ACTIVE;price>100000),stock>0
	node, err := Parse("status==ACTIVE;price>100000,stock>0")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	or, ok := node.(*OrNode)
	if !ok {
		t.Fatalf("expected OrNode at top (AND binds tighter), got %T", node)
	}
	if len(or.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(or.Children))
	}

	and, ok := or.Children[0].(*AndNode)
	if !ok {
		t.Fatalf("expected left child to be AndNode, got %T", or.Children[0])
	}
	if len(and.Children) != 2 {
		t.Fatalf("expected 2 children in AND, got %d", len(and.Children))
	}
}

func TestParseWildcard(t *testing.T) {
	tests := []string{
		"name==Laptop*",
		"name==*Pro",
		"name==*Laptop*",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			node, err := Parse(input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			cmp := node.(*ComparisonNode)
			if cmp.Operator != "==" {
				t.Errorf("expected '==', got %q", cmp.Operator)
			}
			val := cmp.Arguments.(string)
			if !containsWildcard(val) {
				t.Errorf("expected wildcard '*' in value, got %q", val)
			}
		})
	}
}

func TestParseMultipleAnd(t *testing.T) {
	node, err := Parse("a==1;b==2;c==3")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	and, ok := node.(*AndNode)
	if !ok {
		t.Fatalf("expected AndNode, got %T", node)
	}
	if len(and.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(and.Children))
	}
}

func TestParseMultipleOr(t *testing.T) {
	node, err := Parse("a==1,b==2,c==3")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	or, ok := node.(*OrNode)
	if !ok {
		t.Fatalf("expected OrNode, got %T", node)
	}
	if len(or.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(or.Children))
	}
}

func TestParseInvalid(t *testing.T) {
	invalid := []string{
		"invalid",
		"status=in=ACTIVE",
		"==",
		"=",
	}

	for _, input := range invalid {
		t.Run(input, func(t *testing.T) {
			_, err := Parse(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
		})
	}
}

func TestParseWhitespace(t *testing.T) {
	tests := []string{
		" status==ACTIVE ",
		"status == ACTIVE",
		"status==ACTIVE ; price>=100000",
		"status == ACTIVE , status == PENDING",
		" ( status == ACTIVE ) ",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := Parse(input)
			if err != nil {
				t.Errorf("unexpected error for %q: %v", input, err)
			}
		})
	}
}

func TestParseMaxLength(t *testing.T) {
	value := strings.Repeat("a", maxFilterLength-len("name=="))
	if _, err := Parse("name==" + value); err != nil {
		t.Fatalf("filter at max length should parse: %v", err)
	}
	over := strings.Repeat("a", maxFilterLength-len("name==")+1)
	if _, err := Parse("name==" + over); err == nil {
		t.Fatal("expected error for filter exceeding max length")
	}
}

func TestParseMaxParenDepth(t *testing.T) {
	nested := func(n int) string {
		return strings.Repeat("(", n) + "name==x" + strings.Repeat(")", n)
	}
	if _, err := Parse(nested(maxParenDepth)); err != nil {
		t.Fatalf("paren depth %d should be accepted: %v", maxParenDepth, err)
	}
	if _, err := Parse(nested(maxParenDepth + 1)); err == nil {
		t.Fatal("expected error for paren depth exceeding maximum")
	}
}

func TestParseMaxListValues(t *testing.T) {
	list := func(n int) string {
		return "status=in=(" + strings.TrimSuffix(strings.Repeat("v,", n), ",") + ")"
	}
	if _, err := Parse(list(maxListValues)); err != nil {
		t.Fatalf("list of %d values should be accepted: %v", maxListValues, err)
	}
	if _, err := Parse(list(maxListValues + 1)); err == nil {
		t.Fatal("expected error for list exceeding maximum values")
	}
}

func containsDot(s string) bool {
	for _, ch := range s {
		if ch == '.' {
			return true
		}
	}
	return false
}

func containsWildcard(s string) bool {
	for _, ch := range s {
		if ch == '*' {
			return true
		}
	}
	return false
}
