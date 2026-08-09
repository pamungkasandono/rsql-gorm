# rsql-gorm

RQL-style (Resource Query Language) filter builder for [GORM](https://gorm.io). Parse a compact query string into GORM `Where` clauses with automatic joins for relations — without string concatenation or SQL injection.

## Install

```bash
go get github.com/pamungkasandono/rsql-gorm
```

Requires Go 1.26+ and GORM v1.

## Quick start

```go
package main

import (
    "fmt"

    "github.com/pamungkasandono/rsql-gorm"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

type User struct {
    ID   uint   `gorm:"primaryKey"`
    Name string
    Role Role
}

func (User) TableName() string { return "users" }

type Role struct {
    ID   uint   `gorm:"primaryKey"`
    Name string
}

func (Role) TableName() string { return "roles" }

func main() {
    db, _ := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})

    // 1. Parse the RSQL filter string into an AST
    node, err := rsql.Parse(`name==John*;role.name==admin`)
    if err != nil {
        panic(err)
    }

    // 2. Build a *gorm.DB with WHERE clauses + JOINs applied
    query, err := rsql.BuildQuery(db, node, User{})
    if err != nil {
        panic(err)
    }

    var users []User
    query.Find(&users) // SELECT * FROM users
                      //   LEFT JOIN roles ON ...
                      //   WHERE users.name ILIKE 'John%' AND roles.name = 'admin'
    fmt.Println(users)
}
```

## Quick start with pagination

Same filter, plus sort and paging. `BuildPageableQuery` snapshots the conditions, so `Count` and `Find` each run on a fresh statement (GORM's `Count` injects a `SELECT count(*)` that would otherwise pollute the next `Find`).

```go
p, err := rsql.ParseListParams(
    "name==John*;role.name==admin", // filter
    "name:desc",                    // sort
    "2",                            // page
    "10",                           // page size
)
if err != nil {
    panic(err)
}

pq, err := rsql.BuildPageableQuery(db, p, User{})
if err != nil {
    panic(err)
}

var total int64
pq.NewQuery().Count(&total)

var users []User
pq.NewQuery().Limit(pq.Limit).Offset(pq.Offset).Find(&users)

fmt.Println(total, users)
```

`ParseListParams` is the HTTP query-params flow — filter, sort, page and page size come in as strings. Page and page size must be digits only (empty means default); anything else returns an error instead of silently clamping. When building `Params` programmatically (not from HTTP), construct it directly with a parsed `Node`, `[]Sort` and `Pagination` instead. `pq.Page`, `pq.Limit` and `pq.Offset` are the sanitized pagination values — return them in your response for the client to render pagination controls.

If the total count isn't needed, apply the same filter + sort + pagination in one call with `BuildQueryWithParams` and go straight to `Find`:

```go
query, _ := rsql.BuildQueryWithParams(db, p, User{})
query.Find(&users)
```

Each step is also available separately — `BuildQuery` (filter + joins), `ApplySort` (ORDER BY, nested supported), `ApplyPagination` (clamped LIMIT/OFFSET).

## Query syntax

Combined filters use the RSQL convention:

| Token | Meaning            | Example                                   |
|-------|--------------------|-------------------------------------------|
| `;`   | AND                | `status==ACTIVE;price>=1000`             |
| `,`   | OR                 | `status==ACTIVE,status==PENDING`         |
| `(...)` | Grouping         | `status==ACTIVE;(price>1000,price<500)`  |

### Operators

| Operator | SQL          | Example                                   |
|----------|--------------|-------------------------------------------|
| `==`     | `=`, `ILIKE` | `name==Laptop*` (wildcard `*` → `ILIKE`)  |
| `!=`     | `<>`         | `status!=ACTIVE`                          |
| `>`      | `>`          | `price>1000`                              |
| `>=`     | `>=`         | `price>=1000`                             |
| `<`      | `<`          | `price<1000`                              |
| `<=`     | `<=`         | `price<=1000`                             |
| `=in=`   | `IN`         | `status=in=(ACTIVE,PENDING)`             |
| `=out=`  | `NOT IN`     | `status=out=(DELETED,ARCHIVED)`          |

### Relations & joins

Dot-separated selectors traverse struct relations. The builder resolves the GORM `foreignKey`/`references` tags and emits `LEFT JOIN`s automatically, using `__`-separated table aliases to avoid collisions.

```go
node, _ := rsql.Parse(`roles.role.name==operator`)
```

```sql
LEFT JOIN user_roles Roles ON Roles.user_id = users.id
LEFT JOIN roles Roles__Role ON Roles__Role.role_id = Roles.id
WHERE Roles__Role.name = 'operator'
```

- Max join depth is 5 (safe-guarded; configurable later).
- `!=` and `=out=` on a `has-many` relation generate a `NOT IN (SELECT ...)` subquery instead of a naive join, so results are correct when the root has no matching children.

```go
node, _ := rsql.Parse(`roles.RoleName!=operator`)
// WHERE users.id NOT IN (
//   SELECT t0.user_id FROM user_roles t0 WHERE t0.role_name = 'operator')
```

## API reference

| Symbol                       | Description                                        |
|------------------------------|----------------------------------------------------|
| `Parse(input string)`        | Parse an RSQL string into a `Node` AST. `""` → `nil` |
| `BuildQuery(db, node, model)`| Apply the AST to a `*gorm.DB` (validates + joins)   |
| `ApplySort(db, sorts, model)`| Validate fields + apply `ORDER BY` (nested supported) |
| `ApplyPagination(db, pagination)` | Apply clamped `LIMIT`/`OFFSET`                 |
| `BuildQueryWithParams(db, params, model)` | Apply filter + sort + pagination in one call |
| `BuildPageableQuery(db, params, model)` | Filter + sort + sanitized pagination; `NewQuery()` for Count and Find |
| `ParseSort(raw string)`      | Parse `field:desc,field2:asc` into `[]Sort`         |
| `ParseListParams(...)`       | Parse filter + sort + page + page size into `Params` |
| `Params`                     | `{ Pagination, Filter Node, Sorts []Sort }`          |
| `Pagination.Sanitize()`      | Clamp page/limit; returns `(page, limit, offset)`    |
| `Node`                       | AST: `ComparisonNode`, `AndNode`, `OrNode`           |
| `DefaultLimit` / `MaxLimit`  | Pagination bounds (`10` / `1000`)                    |

`Parse` and `BuildQuery` are split deliberately: parse once, reuse the AST for multiple connections or cache it.

## Testing

```bash
go test ./...
```

Tests run against GORM in `DryRun` mode (no database required) using the pure-Go `github.com/glebarez/sqlite` driver.

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## License

MIT.
