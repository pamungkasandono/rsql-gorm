package rsql

import (
	"fmt"
	"reflect"
	"strings"
)

const defaultMaxJoinDepth = 5

type Validator struct {
	rootModel    reflect.Type
	maxJoinDepth int
}

func (v *Validator) Validate(node Node) error {
	if node == nil {
		return nil
	}
	return v.validateNode(node)
}

func (v *Validator) validateNode(node Node) error {
	switch n := node.(type) {
	case *ComparisonNode:
		return v.validateSelector(n.Selector)
	case *AndNode:
		for _, child := range n.Children {
			if err := v.validateNode(child); err != nil {
				return err
			}
		}
	case *OrNode:
		for _, child := range n.Children {
			if err := v.validateNode(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (v *Validator) validateSelector(selector string) error {
	segments := strings.Split(selector, ".")

	if len(segments) < 2 {
		if _, found := findFieldByFold(v.rootModel, segments[0]); !found {
			return fmt.Errorf("field %q not found on %s in selector %q", segments[0], typeName(v.rootModel), selector)
		}
		return nil
	}

	depth := len(segments) - 1
	if depth > v.maxJoinDepth {
		return fmt.Errorf("join depth %d exceeds maximum %d for selector %q", depth, v.maxJoinDepth, selector)
	}

	currentType := v.rootModel
	for i := 0; i < len(segments)-1; i++ {
		field, found := findFieldByFold(currentType, segments[i])
		if !found {
			return fmt.Errorf("relation %q not found on %s in selector %q", segments[i], typeName(currentType), selector)
		}

		ft := derefType(field.Type)
		if !isStructOrSliceOfStruct(ft) {
			return fmt.Errorf("field %q is not a relation (must be struct or slice of struct) in selector %q", segments[i], selector)
		}

		if ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
			ft = ft.Elem()
		}
		currentType = ft
	}

	return nil
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	return t
}

func isStructOrSliceOfStruct(t reflect.Type) bool {
	if t.Kind() == reflect.Struct {
		return true
	}
	if (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) && t.Elem().Kind() == reflect.Struct {
		return true
	}
	return false
}

func typeName(t reflect.Type) string {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
