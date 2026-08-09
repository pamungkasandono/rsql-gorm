package rsql

import (
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
)

type QueryBuilder struct {
	rootModel    reflect.Type
	rootTable    string
	rootPK       string
	maxJoinDepth int
	joins        []string
	joinSeen     map[string]bool
}

type resolvedSelector struct {
	alias     string
	column    string
	steps     []relStep
	isHasMany bool
}

type relStep struct {
	table     string
	alias     string
	fk        string
	ref       string
	joinSQL   string
	isHasMany bool
}

func BuildQuery(db *gorm.DB, node Node, rootModel any) (*gorm.DB, error) {
	if node == nil {
		return db, nil
	}

	qb, err := newQueryBuilder(rootModel)
	if err != nil {
		return nil, err
	}

	v := &Validator{
		rootModel:    qb.rootModel,
		maxJoinDepth: qb.maxJoinDepth,
	}
	if err := v.Validate(node); err != nil {
		return nil, err
	}

	return qb.build(db, node)
}

func newQueryBuilder(rootModel any) (*QueryBuilder, error) {
	modelType := reflect.TypeOf(rootModel)
	for modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}
	if modelType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("filter: root model must be a struct, got %s", modelType.Kind())
	}

	rootTable := tableNameOf(rootModel)
	if rootTable == "" {
		return nil, fmt.Errorf("filter: root model must implement TableName()")
	}

	return &QueryBuilder{
		rootModel:    modelType,
		rootTable:    rootTable,
		rootPK:       findPKColumn(modelType),
		maxJoinDepth: defaultMaxJoinDepth,
		joinSeen:     make(map[string]bool),
	}, nil
}

func (qb *QueryBuilder) build(db *gorm.DB, node Node) (*gorm.DB, error) {
	clause, args, err := qb.buildClause(node)
	if err != nil {
		return nil, err
	}

	for _, j := range qb.joins {
		db = db.Joins(j)
	}

	if clause != "" {
		db = db.Where(clause, args...)
	}

	return db, nil
}

func (qb *QueryBuilder) buildClause(node Node) (string, []any, error) {
	switch n := node.(type) {
	case *ComparisonNode:
		return qb.buildComparison(n)
	case *AndNode:
		return qb.combineChildren(n.Children, "AND")
	case *OrNode:
		return qb.combineChildren(n.Children, "OR")
	default:
		return "", nil, nil
	}
}

func (qb *QueryBuilder) combineChildren(children []Node, op string) (string, []any, error) {
	if len(children) == 0 {
		return "", nil, nil
	}

	var clauses []string
	var allArgs []any

	for _, child := range children {
		c, args, err := qb.buildClause(child)
		if err != nil {
			return "", nil, err
		}
		if c == "" {
			continue
		}
		clauses = append(clauses, c)
		allArgs = append(allArgs, args...)
	}

	if len(clauses) == 0 {
		return "", nil, nil
	}
	if len(clauses) == 1 {
		return clauses[0], allArgs, nil
	}

	return "(" + strings.Join(clauses, " "+op+" ") + ")", allArgs, nil
}

func (qb *QueryBuilder) buildComparison(n *ComparisonNode) (string, []any, error) {
	segments := strings.Split(n.Selector, ".")

	if len(segments) == 1 {
		col := findColumnByPath(qb.rootModel, segments)
		return buildOperatorClause(qb.rootTable+"."+col, n.Operator, n.Arguments)
	}

	resolved, err := qb.resolveSelector(segments)
	if err != nil {
		return "", nil, err
	}

	if (n.Operator == "!=" || n.Operator == "=out=") && resolved.isHasMany {
		return qb.negatedHasMany(resolved, n)
	}

	qb.addJoins(resolved.steps)
	col := resolved.alias + "." + resolved.column
	return buildOperatorClause(col, n.Operator, n.Arguments)
}

func (qb *QueryBuilder) resolveSelector(segments []string) (*resolvedSelector, error) {
	depth := len(segments) - 1

	if depth > qb.maxJoinDepth {
		return nil, fmt.Errorf("join depth %d exceeds maximum %d for selector %q", depth, qb.maxJoinDepth, strings.Join(segments, "."))
	}

	currentType := qb.rootModel
	parentAlias := qb.rootTable
	var steps []relStep
	var pathParts []string
	isHasMany := false

	for i := 0; i < depth; i++ {
		seg := segments[i]
		field, found := findFieldByFold(currentType, seg)
		if !found {
			return nil, fmt.Errorf("relation %q not found on %s in %q", seg, typeName(currentType), strings.Join(segments, "."))
		}

		ft := field.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		hasMany := ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array
		if hasMany {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			return nil, fmt.Errorf("field %q is not a valid relation", seg)
		}

		childTable := tableNameOfType(ft)

		tag := field.Tag.Get("gorm")
		fkField := parseGormTagKey(tag, "foreignKey")
		refField := parseGormTagKey(tag, "references")
		if refField == "" {
			refField = "ID"
		}

		pathParts = append(pathParts, field.Name)
		alias := strings.Join(pathParts, "__")

		var fkCol, refCol string
		if hasMany {
			if fkField == "" {
				fkField = currentType.Name() + "ID"
			}
			fkCol = findColumnByField(ft, fkField)
			refCol = findColumnByField(currentType, refField)
		} else {
			if fkField == "" {
				fkField = ft.Name() + "ID"
			}
			fkCol = findColumnByField(currentType, fkField)
			refCol = findColumnByField(ft, refField)
		}

		joinSQL := fmt.Sprintf("LEFT JOIN %s %s ON %s.%s = %s.%s",
			childTable, alias, alias, fkCol, parentAlias, refCol)

		steps = append(steps, relStep{
			table:     childTable,
			alias:     alias,
			fk:        fkCol,
			ref:       refCol,
			joinSQL:   joinSQL,
			isHasMany: hasMany,
		})

		currentType = ft
		parentAlias = alias
		isHasMany = hasMany
	}

	lastSeg := segments[depth]
	lastField, found := findFieldByFold(currentType, lastSeg)
	if !found {
		return nil, fmt.Errorf("field %q not found on %s in %q", lastSeg, typeName(currentType), strings.Join(segments, "."))
	}
	colName := findColumnByField(currentType, lastField.Name)

	return &resolvedSelector{
		alias:     parentAlias,
		column:    colName,
		steps:     steps,
		isHasMany: isHasMany,
	}, nil
}

func (qb *QueryBuilder) addJoins(steps []relStep) {
	for _, s := range steps {
		if !qb.joinSeen[s.alias] {
			qb.joinSeen[s.alias] = true
			qb.joins = append(qb.joins, s.joinSQL)
		}
	}
}

func (qb *QueryBuilder) negatedHasMany(resolved *resolvedSelector, n *ComparisonNode) (string, []any, error) {
	if len(resolved.steps) == 0 {
		col := resolved.alias + "." + resolved.column
		return buildOperatorClause(col, n.Operator, n.Arguments)
	}

	var subquery strings.Builder

	firstStep := resolved.steps[0]
	subquery.WriteString("SELECT ")
	subquery.WriteString("t0.")
	subquery.WriteString(firstStep.fk)
	subquery.WriteString(" FROM ")
	subquery.WriteString(firstStep.table + " t0")

	for i := 1; i < len(resolved.steps); i++ {
		curr := resolved.steps[i]
		subquery.WriteString(fmt.Sprintf(" JOIN %s t%d", curr.table, i))
		subquery.WriteString(fmt.Sprintf(" ON t%d.%s = t%d.%s", i, curr.ref, i-1, curr.fk))
	}

	innerCol := fmt.Sprintf("t%d.%s", len(resolved.steps)-1, resolved.column)
	subquery.WriteString(" WHERE ")

	var innerArgs []any
	switch n.Operator {
	case "=out=":
		vals, ok := n.Arguments.([]string)
		if !ok {
			return "", nil, fmt.Errorf("=out= requires string list arguments")
		}
		placeholders := make([]string, len(vals))
		innerArgs = make([]any, len(vals))
		for i, v := range vals {
			placeholders[i] = "?"
			innerArgs[i] = v
		}
		subquery.WriteString(innerCol)
		subquery.WriteString(" IN (" + strings.Join(placeholders, ",") + ")")
	default:
		val, ok := n.Arguments.(string)
		if !ok {
			return "", nil, fmt.Errorf("!= requires string arguments")
		}
		clause, args, err := buildEquality(innerCol, val)
		if err != nil {
			return "", nil, err
		}
		subquery.WriteString(clause)
		innerArgs = args
	}

	fullPK := qb.rootTable + "." + qb.rootPK
	return fmt.Sprintf("%s NOT IN (%s)", fullPK, subquery.String()), innerArgs, nil
}

// -- reflection helpers --

func findFieldByFold(t reflect.Type, name string) (reflect.StructField, bool) {
	return t.FieldByNameFunc(func(fn string) bool {
		return strings.EqualFold(fn, name)
	})
}

func findColumnByField(t reflect.Type, fieldName string) string {
	field, found := findFieldByFold(t, fieldName)
	if !found {
		return toSnakeCase(fieldName)
	}
	if col := parseGormTagKey(field.Tag.Get("gorm"), "column"); col != "" {
		return col
	}
	return toSnakeCase(field.Name)
}

func findColumnByPath(t reflect.Type, segments []string) string {
	currentType := t
	for i := 0; i < len(segments)-1; i++ {
		field, found := findFieldByFold(currentType, segments[i])
		if !found {
			return segments[len(segments)-1]
		}
		ft := field.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Array {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			return segments[len(segments)-1]
		}
		currentType = ft
	}
	field, found := findFieldByFold(currentType, segments[len(segments)-1])
	if found {
		if col := parseGormTagKey(field.Tag.Get("gorm"), "column"); col != "" {
			return col
		}
		return toSnakeCase(field.Name)
	}
	return toSnakeCase(segments[len(segments)-1])
}

func findPKColumn(t reflect.Type) string {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("gorm")
		if strings.Contains(tag, "primaryKey") {
			if col := parseGormTagKey(tag, "column"); col != "" {
				return col
			}
			return toSnakeCase(f.Name)
		}
	}
	return "id"
}

func parseGormTagKey(tag, key string) string {
	for _, part := range strings.Split(tag, ";") {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == key {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}

func tableNameOfType(t reflect.Type) string {
	instance := reflect.New(t).Interface()
	if named, ok := instance.(interface{ TableName() string }); ok {
		return named.TableName()
	}
	return toSnakeCase(t.Name())
}

// -- operator clause builders --

func buildOperatorClause(column string, op string, args any) (string, []any, error) {
	switch op {
	case "==":
		return buildEquality(column, args)
	case "!=":
		return buildNotEqual(column, args)
	case ">":
		return column + " > ?", []any{args}, nil
	case ">=":
		return column + " >= ?", []any{args}, nil
	case "<":
		return column + " < ?", []any{args}, nil
	case "<=":
		return column + " <= ?", []any{args}, nil
	case "=in=":
		return buildIn(column, args)
	case "=out=":
		return buildNotIn(column, args)
	default:
		return column + " = ?", []any{args}, nil
	}
}

func buildEquality(column string, args any) (string, []any, error) {
	val, ok := args.(string)
	if !ok {
		return column + " = ?", []any{args}, nil
	}

	if needsPatternConversion(val) {
		return column + " ILIKE ? ESCAPE '\\'", []any{wildcardPattern(val)}, nil
	}

	if isNullKeyword(val) {
		return column + " IS NULL", nil, nil
	}

	return column + " = ?", []any{val}, nil
}

// needsPatternConversion reports whether a value must go through the LIKE
// pipeline: it contains a wildcard (*) or an escape sequence (\% or \_), which
// changes the effective literal value.
func needsPatternConversion(val string) bool {
	for i := 0; i < len(val); i++ {
		if val[i] == '*' {
			return true
		}
		if val[i] == '\\' && i+1 < len(val) && (val[i+1] == '%' || val[i+1] == '_') {
			return true
		}
	}
	return false
}

// wildcardPattern converts an RSQL value into a SQL LIKE pattern. Only `*` is
// a wildcard (mapped to SQL `%`); `%`, `_` and `\` are always literal, so they
// are backslash-escaped in the pattern. A backslash escapes the next character:
// `\*` stays a literal asterisk, `\%` and `\_` stay literal, and `\\` is a
// literal backslash (so `\\*` is a literal backslash followed by a wildcard).
// A backslash before any other character, or at the end of the value, is
// itself a literal backslash.
func wildcardPattern(val string) string {
	var b strings.Builder
	b.Grow(len(val))
	for i := 0; i < len(val); i++ {
		switch {
		case val[i] == '\\' && i+1 < len(val) && val[i+1] == '*':
			b.WriteByte('*')
			i++
		case val[i] == '\\' && i+1 < len(val) && (val[i+1] == '%' || val[i+1] == '_'):
			b.WriteByte('\\')
			b.WriteByte(val[i+1])
			i++
		case val[i] == '\\' && i+1 < len(val) && val[i+1] == '\\':
			b.WriteByte('\\')
			b.WriteByte('\\')
			i++
		case val[i] == '\\':
			b.WriteByte('\\')
			b.WriteByte('\\')
		case val[i] == '*':
			b.WriteByte('%')
		case val[i] == '%' || val[i] == '_':
			b.WriteByte('\\')
			b.WriteByte(val[i])
		default:
			b.WriteByte(val[i])
		}
	}
	return b.String()
}

func buildNotEqual(column string, args any) (string, []any, error) {
	val, ok := args.(string)
	if !ok {
		return column + " <> ?", []any{args}, nil
	}
	if needsPatternConversion(val) {
		return column + " NOT ILIKE ? ESCAPE '\\'", []any{wildcardPattern(val)}, nil
	}
	if isNullKeyword(val) {
		return column + " IS NOT NULL", nil, nil
	}
	return column + " <> ?", []any{val}, nil
}

// isNullKeyword reports whether a string value is exactly "null" (any case),
// ignoring surrounding whitespace. Wildcard-based searches are unaffected.
func isNullKeyword(val string) bool {
	return strings.EqualFold(strings.TrimSpace(val), "null")
}

func buildIn(column string, args any) (string, []any, error) {
	vals, ok := args.([]string)
	if !ok {
		return "", nil, fmt.Errorf("=in= requires list arguments")
	}
	placeholders := make([]string, len(vals))
	parsedArgs := make([]any, len(vals))
	for i, v := range vals {
		placeholders[i] = "?"
		parsedArgs[i] = v
	}
	return column + " IN (" + strings.Join(placeholders, ",") + ")", parsedArgs, nil
}

func buildNotIn(column string, args any) (string, []any, error) {
	vals, ok := args.([]string)
	if !ok {
		return "", nil, fmt.Errorf("=out= requires list arguments")
	}
	placeholders := make([]string, len(vals))
	parsedArgs := make([]any, len(vals))
	for i, v := range vals {
		placeholders[i] = "?"
		parsedArgs[i] = v
	}
	return column + " NOT IN (" + strings.Join(placeholders, ",") + ")", parsedArgs, nil
}

func tableNameOf(model any) string {
	if named, ok := model.(interface{ TableName() string }); ok {
		return named.TableName()
	}
	return ""
}

func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteByte(byte(r + 32))
		} else {
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}
