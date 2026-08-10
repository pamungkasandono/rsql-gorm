package rsql

import (
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

type Aliases map[string]string

func WithAliases(db *gorm.DB, model any, aliases Aliases) (*gorm.DB, error) {
	if err := validateRootModel(model); err != nil {
		return nil, err
	}
	rootType := derefPtrType(reflect.TypeOf(model))
	if len(aliases) > 0 {
		if err := validateAliases(rootType, aliases); err != nil {
			return nil, err
		}
	}
	return db.Set(aliasKey(rootType), aliases), nil
}

func resolveAliases(selector string, aliases Aliases) (string, error) {
	if len(aliases) == 0 {
		return selector, nil
	}
	var bestKey, bestVal string
	bestSegments := -1
	for k, v := range aliases {
		if k == "" {
			return "", fmt.Errorf("alias: empty key")
		}
		if v == "" {
			return "", fmt.Errorf("alias %q: empty value", k)
		}
		if selector == k || strings.HasPrefix(selector, k+".") {
			segCount := len(strings.Split(k, "."))
			if segCount > bestSegments {
				bestSegments = segCount
				bestKey = k
				bestVal = v
			}
		}
	}
	if bestSegments < 0 {
		return selector, nil
	}
	rest := strings.TrimPrefix(selector, bestKey)
	rest = strings.TrimPrefix(rest, ".")
	if rest == "" {
		return bestVal, nil
	}
	return bestVal + "." + rest, nil
}

func rewriteNode(node Node, aliases Aliases) (Node, error) {
	if node == nil || len(aliases) == 0 {
		return node, nil
	}
	switch n := node.(type) {
	case *ComparisonNode:
		sel, err := resolveAliases(n.Selector, aliases)
		if err != nil {
			return nil, err
		}
		return &ComparisonNode{Selector: sel, Operator: n.Operator, Arguments: n.Arguments}, nil
	case *AndNode:
		children := make([]Node, len(n.Children))
		for i, c := range n.Children {
			rw, err := rewriteNode(c, aliases)
			if err != nil {
				return nil, err
			}
			children[i] = rw
		}
		return &AndNode{Children: children}, nil
	case *OrNode:
		children := make([]Node, len(n.Children))
		for i, c := range n.Children {
			rw, err := rewriteNode(c, aliases)
			if err != nil {
				return nil, err
			}
			children[i] = rw
		}
		return &OrNode{Children: children}, nil
	default:
		return node, nil
	}
}

func validateRootModel(model any) error {
	modelType := reflect.TypeOf(model)
	for modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	if modelType.Kind() != reflect.Struct {
		return fmt.Errorf("rsql: model must be a struct, got %s", modelType.Kind())
	}
	if tableNameOf(model) == "" {
		return fmt.Errorf("rsql: model must implement TableName()")
	}
	return nil
}

func validateAliases(rootType reflect.Type, aliases Aliases) error {
	for publicName, internalPath := range aliases {
		if publicName == "" {
			return fmt.Errorf("alias: empty key")
		}
		if internalPath == "" {
			return fmt.Errorf("alias %q: empty value", publicName)
		}
		if err := validateFieldPath(rootType, internalPath); err != nil {
			return fmt.Errorf("alias %q: %w", publicName, err)
		}
	}
	return nil
}

func validateFieldPath(rootType reflect.Type, path string) error {
	segments := strings.Split(path, ".")
	currentType := rootType
	for i, seg := range segments {
		field, found := findFieldByFold(currentType, seg)
		if !found {
			return fmt.Errorf("field %q not found on %s", seg, typeName(currentType))
		}
		if i < len(segments)-1 {
			ft := derefType(field.Type)
			if !isStructOrSliceOfStruct(ft) {
				return fmt.Errorf("field %q is not a relation", seg)
			}
			if ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
				ft = ft.Elem()
			}
			currentType = ft
		}
	}
	return nil
}

func aliasesFor(db *gorm.DB, model any) Aliases {
	rootType := derefPtrType(reflect.TypeOf(model))
	v, ok := db.Get(aliasKey(rootType))
	if !ok {
		return nil
	}
	a, _ := v.(Aliases)
	return a
}

func aliasKey(t reflect.Type) string {
	return "rsql:aliases:" + t.PkgPath() + "." + t.Name()
}

func derefPtrType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
