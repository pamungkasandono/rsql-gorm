package rsql

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ApplySort validates each sort field against the root model and applies
// ORDER BY clauses. Dotted selectors (e.g. role.name) are resolved through
// the same join machinery used by BuildQuery, so nested sorts produce the
// required LEFT JOINs automatically. Unknown fields return an error instead
// of being interpolated into SQL, guarding against injection.
func ApplySort(db *gorm.DB, sorts []Sort, model any) (*gorm.DB, error) {
	if len(sorts) == 0 {
		return db, nil
	}

	qb, err := newQueryBuilder(model)
	if err != nil {
		return nil, err
	}

	aliases := aliasesFor(db, model)

	for _, s := range sorts {
		field, err := resolveAliases(s.Field, aliases)
		if err != nil {
			return nil, fmt.Errorf("sort: %w", err)
		}
		column, steps, err := qb.resolveSortSelector(field)
		if err != nil {
			return nil, err
		}
		qb.addJoins(steps)

		dir := "ASC"
		if s.Desc {
			dir = "DESC"
		}
		db = db.Order(column + " " + dir)
	}

	for _, j := range qb.joins {
		db = db.Joins(j)
	}

	return db, nil
}

func (qb *queryBuilder) resolveSortSelector(selector string) (string, []relStep, error) {
	segments := strings.Split(selector, ".")

	if len(segments) == 1 {
		field, found := findFieldByFold(qb.rootModel, segments[0])
		if !found {
			return "", nil, fmt.Errorf("sort: field %q not found on %s", segments[0], typeName(qb.rootModel))
		}
		return qb.rootTable + "." + findColumnByField(qb.rootModel, field.Name), nil, nil
	}

	resolved, err := qb.resolveSelector(segments)
	if err != nil {
		return "", nil, fmt.Errorf("sort: %w", err)
	}

	return resolved.alias + "." + resolved.column, resolved.steps, nil
}

// ApplyPagination clamps page and limit via Pagination.Sanitize and applies
// LIMIT/OFFSET to the query.
func ApplyPagination(db *gorm.DB, p Pagination) *gorm.DB {
	_, limit, offset := p.Sanitize()
	return db.Limit(limit).Offset(offset)
}

// PagedQuery snapshots the built filter+sort conditions so every NewQuery call
// produces a fresh *gorm.DB with the same conditions but a clean statement.
// This avoids GORM's Count (which injects a SELECT count(*)) polluting a later
// Find, and guarantees the Count is always derived from the same conditions as
// the rows query.
type PagedQuery struct {
	base    *gorm.DB
	model   any
	clauses map[string]clause.Clause
	joins   []string
	Page    int
	Limit   int
	Offset  int
}

// NewQuery returns a fresh *gorm.DB with the filter+sort conditions applied and
// no LIMIT/OFFSET. Call it once for Count and again for Limit/Offset+Find.
func (pq *PagedQuery) NewQuery() *gorm.DB {
	fresh := pq.base.Session(&gorm.Session{NewDB: true}).Model(pq.model)
	for name, c := range pq.clauses {
		fresh.Statement.Clauses[name] = c
	}
	for _, j := range pq.joins {
		fresh = fresh.Joins(j)
	}
	return fresh
}

// BuildPageableQuery applies filter and sort, returning a PagedQuery with the
// sanitized pagination values and a fresh-query factory instead of a single
// *gorm.DB. Prefer this over BuildQueryWithParams when the total row count is
// also needed.
func BuildPageableQuery(db *gorm.DB, params *Params, model any) (*PagedQuery, error) {
	query, err := BuildQuery(db, params.Filter, model)
	if err != nil {
		return nil, err
	}

	query, err = ApplySort(query, params.Sorts, model)
	if err != nil {
		return nil, err
	}

	clauses := make(map[string]clause.Clause, len(query.Statement.Clauses))
	for name, c := range query.Statement.Clauses {
		clauses[name] = c
	}
	joins := make([]string, 0, len(query.Statement.Joins))
	for _, j := range query.Statement.Joins {
		joins = append(joins, j.Name)
	}

	page, limit, offset := params.Pagination.Sanitize()
	return &PagedQuery{
		base:    db,
		model:   model,
		clauses: clauses,
		joins:   joins,
		Page:    page,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// BuildQueryWithParams applies filter (BuildQuery), then sort (ApplySort) and
// pagination (ApplyPagination) in a single call.
func BuildQueryWithParams(db *gorm.DB, params *Params, model any) (*gorm.DB, error) {
	pq, err := BuildPageableQuery(db, params, model)
	if err != nil {
		return nil, err
	}
	return pq.NewQuery().Limit(pq.Limit).Offset(pq.Offset), nil
}
