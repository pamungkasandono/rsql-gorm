# rsql-gorm

[![CI](https://github.com/pamungkasandono/rsql-gorm/actions/workflows/ci.yml/badge.svg)](https://github.com/pamungkasandono/rsql-gorm/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/dl/)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/pamungkasandono/rsql-gorm)](https://pkg.go.dev/github.com/pamungkasandono/rsql-gorm)
[![Release](https://img.shields.io/github/v/release/pamungkasandono/rsql-gorm?sort=semver)](https://github.com/pamungkasandono/rsql-gorm/releases)
[![License](https://img.shields.io/github/license/pamungkasandono/rsql-gorm)](https://github.com/pamungkasandono/rsql-gorm/blob/main/LICENSE)

RQL-style (Resource Query Language) filter builder for [GORM](https://gorm.io). Parse a compact query string into GORM `Where` clauses with automatic joins for relations, without string concatenation or SQL injection.

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
                      //   WHERE users.name ILIKE 'John%' ESCAPE '\' AND roles.name = 'admin'
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

`ParseListParams` is the HTTP query-params flow: filter, sort, page and page size come in as strings. Page and page size must be digits only (empty means default); anything else returns an error instead of silently clamping. When building `Params` programmatically (not from HTTP), construct it directly with a parsed `Node`, `[]Sort` and `Pagination` instead. `pq.Page`, `pq.Limit` and `pq.Offset` are the sanitized pagination values, return them in your response for the client to render pagination controls.

If the total count isn't needed, apply the same filter + sort + pagination in one call with `BuildQueryWithParams` and go straight to `Find`:

```go
query, _ := rsql.BuildQueryWithParams(db, p, User{})
query.Find(&users)
```

Each step is also available separately: `BuildQuery` (filter + joins), `ApplySort` (ORDER BY, nested supported), `ApplyPagination` (clamped LIMIT/OFFSET).

## Query syntax

Combined filters use the RSQL convention:

| Token   | Meaning  | Example                                 |
| ------- | -------- | --------------------------------------- |
| `;`     | AND      | `status==ACTIVE;price>=1000`            |
| `,`     | OR       | `status==ACTIVE,status==PENDING`        |
| `(...)` | Grouping | `status==ACTIVE;(price>1000,price<500)` |

### Operators

| Operator | SQL                              | Example                                                                                      |
| -------- | -------------------------------- | -------------------------------------------------------------------------------------------- |
| `==`     | `=`, `ILIKE`, `IS NULL`          | `name==Laptop*` (wildcard `*` → `ILIKE`); `name==null` → `IS NULL`                           |
| `!=`     | `<>`, `NOT ILIKE`, `IS NOT NULL` | `name!=Laptop*` (wildcard `*` → `NOT ILIKE`); `status!=ACTIVE`; `name!=null` → `IS NOT NULL` |
| `>`      | `>`                              | `price>1000`                                                                                 |
| `>=`     | `>=`                             | `price>=1000`                                                                                |
| `<`      | `<`                              | `price<1000`                                                                                 |
| `<=`     | `<=`                             | `price<=1000`                                                                                |
| `=in=`   | `IN`                             | `status=in=(ACTIVE,PENDING)`                                                                 |
| `=out=`  | `NOT IN`                         | `status=out=(DELETED,ARCHIVED)`                                                              |

`==null` / `!=null` (case-insensitive) map to `IS NULL` / `IS NOT NULL`. Wildcards disable this: `name==*null*` is a regular `ILIKE` search and `name!=*null*` a `NOT ILIKE` search. In `=in=` / `=out=`, `null` stays a string literal (matching the exact value `'null'`); use `==` / `!=` for null filtering.

Wildcard `*` maps to SQL `%`. Use `\*` for a literal asterisk: `name==a\*b` searches for `a*b`, `name==a*b` searches for any value starting `a` and ending `b`. `%`, `_` and `\` are always literal in wildcard values: `name==*100%*` finds values containing `100%`, `name==*user_name*` values containing `user_name`, and `name==*.com\city\` matches the path `.com\city\`. A backslash escapes the next character, so `\\` is a literal backslash and `\\*` is a literal backslash followed by a wildcard: `name==*.com\city\\*` matches any value containing the path `.com\city\`. `\%` and `\_` are the same as their unescaped forms.

The same rules apply to `!=`, which generates `NOT ILIKE ... ESCAPE '\'` instead of `<>` when the value contains a wildcard or escape sequence. In `=in=` / `=out=`, values stay literal.

ILIKE patterns are emitted with an explicit `ESCAPE '\'` clause so the escaping above behaves the same across dialects instead of relying on the database's default escape character (PostgreSQL/MySQL default to `\`, SQLite has none).

### Relations & joins

Dot-separated selectors traverse struct relations. The builder resolves the GORM `foreignKey`/`references` tags and emits `LEFT JOIN`s automatically, using `__`-separated table aliases to avoid collisions.

Each segment must match a **Go struct field name** (case-insensitive), not the table name, and there is no singular/plural inflection. If the field is `Roles`, use `roles`; if it's `Role`, use `role`. A mismatch fails loudly, e.g. `field "role" not found on User in "role.name"`. This applies to single-segment selectors too: `status==ACTIVE` is only valid when `Status` is a field on the root model, otherwise the query is rejected instead of interpolating the raw name into SQL.

```go
node, _ := rsql.Parse(`roles.role.name==operator`)
```

```sql
LEFT JOIN user_roles Roles ON Roles.user_id = users.id
LEFT JOIN roles Roles__Role ON Roles__Role.role_id = Roles.id
WHERE Roles__Role.name = 'operator'
```

- Max join depth is 5 (safe-guarded; configurable later).
- `!=` and `=out=` on a `has-many` relation generate a `NOT IN (SELECT ...)` subquery instead of a naive join, so results are correct when the root has no matching children. Wildcard values in `!=` are matched with `ILIKE` inside the subquery, and `!=null` with `IS NULL`.

```go
node, _ := rsql.Parse(`roles.RoleName!=st*ff`)
// WHERE users.id NOT IN (
//   SELECT t0.user_id FROM user_roles t0 WHERE t0.role_name ILIKE 'st%ff' ESCAPE '\')
```

### Aliases

RSQL aliases let frontend write filter selectors that match the **JSON response shape**, not the underlying database field names or join structure. Aliases are registered once per root model via `WithAliases` (typically at repository construction) and are **eagerly validated** against the model so typos surface immediately.

```go
db, err := rsql.WithAliases(db, Product{},
    rsql.Aliases{
        "brand":      "Brand",              // direct has-one relation
        "categories": "Cats.Category",      // junction / multi-hop path
        "category":   "Cats.Category",      // multiple aliases for one relation
        "variants":   "Vars",               // direct has-many relation
        "brandLabel": "Brand.Name",         // rename a leaf field
    },
)
if err != nil {
    // invalid path detected immediately fail-fast
}
```

Once registered, the aliased `*gorm.DB` is used normally — every `BuildQuery`, `ApplySort`, `BuildQueryWithParams`, and `BuildPageableQuery` automatically resolves aliases from the same `*gorm.DB` handle. This means a single repository can register aliases for multiple root models:

```go
db, err = rsql.WithAliases(db, Product{}, productAliases)
db, err = rsql.WithAliases(db, Category{}, categoryAliases)
```

#### How aliases work

When a selector is resolved, each alias key is compared **as a dot-separated prefix** of the selector. If the selector equals the key, or starts with `key + "."`, the matching prefix is replaced with the alias **value** the internal Go field path. The remainder of the selector is resolved normally through the GORM model.

Longest-prefix wins when multiple aliases match:

| Selector                 | Aliases                                                    | Resolved to                   |
| ------------------------ | ---------------------------------------------------------- | ----------------------------- |
| `categories.name`        | `"categories"→"Cats.Category"`                             | `Cats.Category.name`          |
| `categories.parent.name` | `"categories"→"Cats"`, `"categories.parent"→"Cats.Parent"` | `Cats.Parent.name` (longer)   |
| `brandLabel`             | `"brandLabel"→"Brand.Name"`                                | `Brand.Name` (leaf rename)    |
| `name`                   | (any aliases)                                              | `name` (no match → unchanged) |

Selectors without an alias match, or internal Go field paths used directly, are resolved as before. Aliases take no effect when registered on a different `*gorm.DB` instance — each `WithAliases` call stores the map in the GORM session.

#### Normal case: direct relation rename

JSON response `brand.name`, Go field `Brand`:

```go
rsql.Aliases{"brand": "Brand"}
// filter: brand.name==Apple  →  SQL: ... Brand.name = ?
// sort:  brand.name:asc      →  SQL: ORDER BY Brand.name ASC
```

#### Junction / multi-hop path

JSON response `categories.name`, DB uses junction table `product_categories` → `categories`:

```go
rsql.Aliases{"categories": "Cats.Category"}
// filter: categories.name==Laptop  →  LEFT JOIN product_categories Cats ...
//                                      LEFT JOIN categories Cats__Category ...
//                                      WHERE Cats__Category.name = ?
```

The alias value `Cats.Category` tells RSQL to walk **two** relation steps. The final field (`name`) is resolved inside the target struct (`Category`).

#### Multiple aliases for one relation

```go
rsql.Aliases{"categories": "Cats.Category", "category": "Cats.Category"}
// categories.name==A  and  category.name==A  both work
```

#### Leaf rename

```go
rsql.Aliases{"brandLabel": "Brand.Name"}
// filter: brandLabel==Apple  →  LEFT JOIN brands Brand ... WHERE Brand.name = ?
```

#### Negated has-many

Aliases don't change the SQL generation semantics: `!=` and `=out=` on a **direct** has-many relation still produce the `NOT IN (SELECT …)` subquery automatically. When the alias target path ends with a has-one or scalar field (e.g. `Cats.Category` where `Category` is has-one), the comparison stays as a normal `!=` / `NOT IN` on the joined column, which is the correct behaviour.

#### Validation

`WithAliases` validates the root model (must be a struct with `TableName()`) and every alias value path against that model. Errors include the offending alias key and the specific field that couldn't be found, so configuration problems are caught before any request runs.

```text
alias "categories": field "Categoryz" not found on aliasProduct
```

## Limits & safety

Filters are fully parameterized (`?` placeholders) and selectors are validated against the model, so values cannot escape into SQL. Input bounds exist so a hostile request cannot exhaust the database or the server:

| Bound             | Value | Enforced when                                   |
| ----------------- | ----- | ----------------------------------------------- |
| `MaxFilterLength` | 8192  | `Parse` rejects filters longer than 8 KB        |
| `MaxParenDepth`   | 100   | `Parse` rejects deeper `(...)` nesting          |
| `MaxListValues`   | 2000  | `Parse` rejects `=in=`/`=out=` lists that large |
| `MaxLimit`        | 1000  | `Pagination.Sanitize` clamps `limit`            |
| `MaxPage`         | 10000 | `Pagination.Sanitize` clamps `page` (OFFSET)    |
| Max join depth    | 5     | `BuildQuery` rejects deeper selectors           |

`BuildQuery` alone applies no `LIMIT`; prefer `BuildPageableQuery` / `BuildQueryWithParams` for request-facing endpoints so results are capped at `MaxLimit`.

## API reference

| Symbol                                    | Description                                                           |
| ----------------------------------------- | --------------------------------------------------------------------- |
| `Parse(input string)`                     | Parse an RSQL string into a `Node` AST. `""` → `nil`                  |
| `BuildQuery(db, node, model)`             | Apply the AST to a `*gorm.DB` (validates + joins)                     |
| `ApplySort(db, sorts, model)`             | Validate fields + apply `ORDER BY` (nested supported)                 |
| `ApplyPagination(db, pagination)`         | Apply clamped `LIMIT`/`OFFSET`                                        |
| `BuildQueryWithParams(db, params, model)` | Apply filter + sort + pagination in one call                          |
| `BuildPageableQuery(db, params, model)`   | Filter + sort + sanitized pagination; `NewQuery()` for Count and Find |
| `ParseSort(raw string)`                   | Parse `field:desc,field2:asc` into `[]Sort`                           |
| `ParseListParams(...)`                    | Parse filter + sort + page + page size into `Params`                  |
| `WithAliases(db, model, aliases)`         | Register aliases against a model for the `*gorm.DB` (eager-validated) |
| `Aliases`                                 | `map[string]string` public path → internal Go field path            |
| `Params`                                  | `{ Pagination, Filter Node, Sorts []Sort }`                           |
| `Pagination.Sanitize()`                   | Clamp page/limit; returns `(page, limit, offset)`                     |
| `Node`                                    | AST: `ComparisonNode`, `AndNode`, `OrNode`                            |
| `DefaultLimit` / `MaxLimit` / `MaxPage`   | Pagination bounds (`10` / `1000` / `10000`)                           |

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
