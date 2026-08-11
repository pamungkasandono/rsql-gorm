# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-08-11

### Changed

- `QueryBuilder` and `Validator` are now unexported internal types. They were
  not part of the documented API and had no exported members users could call.
- `MaxFilterLength`, `MaxParenDepth` and `MaxListValues` are now unexported;
  they are parser-internal DoS limits with no external consumers.
- `Pagination.Sanitize` is now the unexported `sanitize`; it was redundant with
  the already-public `PagedQuery.Page`/`Limit`/`Offset`, which carry the same
  clamped values.
- Added Go doc comments to the public API surface (`Parse`, AST node types,
  `BuildQuery`, `Aliases`, `WithAliases`, `Params`, `Pagination`, `Sort`,
  `ParseSort`, `ParseListParams`).

## [0.3.0] - 2026-08-10

### Added

- `WithAliases(db, model, aliases)` to register public→internal field path
  mappings on a `*gorm.DB` instance. Aliases let frontend filter writers use
  the JSON response shape (e.g. `categories.name==Laptop`) while the library
  internally resolves junction tables and joined paths
  (e.g. `Cats.Category.name`). One model can have multiple aliases per
  relation; longest-dot-segment-prefix decides which alias applies when a
  selector matches more than one. Alias configuration is **eagerly validated**
  against the GORM model so mapping errors are caught immediately at
  repository construction, not at query time.
- `Aliases` type: `map[string]string` where keys are public filter names and
  values are internal dot-separated Go field paths.
- Transparent internal performance cache: root-model metadata (type, table
  name, primary key column) is resolved once per `reflect.Type` and reused
  across requests. All existing query functions (`BuildQuery`, `ApplySort`,
  `BuildQueryWithParams`, `BuildPageableQuery`) benefit from the same cache
  without API changes.
- MIT `LICENSE` and license badge on the README.

## [0.2.0] - 2026-08-10

### Added

- `ApplySort` to apply validated `ORDER BY` clauses, including nested selectors
  (e.g. `Role.Name`) that resolve through the existing join machinery.
- `ApplyPagination` to apply clamped `LIMIT`/`OFFSET` from a `Pagination`.
- `BuildQueryWithParams` to apply filter, sort and pagination in a single call.
- Input bounds to protect against resource-exhaustion (DoS) requests:
  `MaxFilterLength` (8192 bytes) rejects oversized filters in `Parse`,
  `MaxParenDepth` (100) rejects deeply nested grouping that could overflow the
  parser stack, and `MaxListValues` (2000) caps `=in=`/`=out=` argument lists.
  `MaxPage` (10000) clamps the pagination offset in `Pagination.Sanitize`.
- DoS tests: deep parens, over-length filter, oversized argument list, and page
  clamping, alongside the existing injection tests.

### Changed

- `!=` now maps wildcard values (containing `*`, `\%` or `\_`) to
  `NOT ILIKE ... ESCAPE '\'` instead of `<>`, consistent with `==`. The
  negated `has-many` `NOT IN (SELECT ...)` subquery applies the same pattern
  matching and `IS NULL` handling on its inner predicate; the outer `NOT IN`
  inverts it, so `roles.RoleName!=null` still excludes rows that match
  `IS NULL`.
- `resolveSelector` now rejects selectors whose last segment is not a known
  struct field, instead of silently falling back to the raw name. Query
  builder and sort fields must reference Go struct field names; DB column
  names are still derived from `gorm` tags.
- `BuildQuery` extracted its model initialization into `newQueryBuilder`, now
  shared with `ApplySort`.

### Fixed

- `ParseListParams` now rejects non-numeric page and page size values with an
  error, instead of silently clamping them to the defaults.
- Single-segment filter selectors are now validated against the root model
  and rejected when the field is unknown (e.g. `x--==1`, `*==1`). Previously
  they fell back to the raw selector as a column name, which could leak SQL
  comment characters (`-`) or `*` into the generated query. Values remain
  fully parameterized (`?` placeholders), with tests covering injection-style
  payloads across `==`, `!=`, `=in=`, `=out=` and negated `has-many`
  subqueries, plus creative SQL-semantics cases executed against a real
  in-memory SQLite database: empty lists (`=out=()` returns the whole table,
  `=in=()` matches nothing), three-valued NULL logic in `!=`/OR tautologies,
  and `name!=*` (a `NOT ILIKE '%'` that effectively selects NULL rows).

### Added

- `==null` / `!=null` (case-insensitive) now map to `IS NULL` / `IS NOT NULL`
  instead of comparing the literal string. Wildcard values (e.g. `*null*`)
  still generate `ILIKE` searches; `null` inside `=in=` / `=out=` remains a
  string literal.
- `\*` escapes the wildcard so a literal asterisk can be searched: `a\*b`
  becomes the pattern `a*b` instead of `a%b`. An unescaped `*` still maps to
  SQL `%`.
- `%` and `_` are always literal: `*100%*` searches for values containing
  `100%`, `*user_name*` for values containing `user_name`. Their SQL `LIKE`
  wildcard meaning is escaped automatically; `\%` and `\_` work the same as
  their unescaped forms.
- Backslashes in wildcard values are literal: `*.com\city\` matches the path
  `.com\city\`, `\\` matches a single literal backslash. `\\*` is a literal
  backslash followed by a wildcard (e.g. `*.com\city\\*` matches any value
  containing the path `.com\city\`); `\*` alone stays a literal asterisk and
  `\%`/`\_` work the same as their unescaped forms.
- `ILIKE` patterns now carry an explicit `ESCAPE '\'` clause so escaping
  behaves the same across dialects instead of relying on the database's
  default escape character.

## [0.1.0] - 2026-08-09

### Added

- RSQL parser supporting `==`, `!=`, `>`, `>=`, `<`, `<=`, `=in=`, `=out=`
  operators with `;` (AND), `,` (OR) and `(...)` grouping.
- Wildcard `*` support in `==` values mapped to `ILIKE` (PostgreSQL).
- `BuildQuery` to apply a parsed AST to a `*gorm.DB`, including:
  - automatic `LEFT JOIN` resolution for nested relations via GORM
    `foreignKey`/`references` tags;
  - `__`-separated table aliases to avoid join collisions;
  - `NOT IN (SELECT ...)` subqueries for negated `has-many` relations.
- Selector validation against the root model with a configurable max join depth.
- `ParseListParams`, `ParseSort` and `Pagination.Sanitize` for list/table
  endpoints (filter + sort + pagination).
- Unit tests for parser, validator, params and query builder (GORM `DryRun`
  mode, pure-Go SQLite driver, no database required).
- CI workflow (GitHub Actions): `go vet` + `go test -race` on Go 1.26.
- `README.md`, `CHANGELOG.md`, `.gitignore`.

[Unreleased]: https://github.com/pamungkasandono/rsql-gorm/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/pamungkasandono/rsql-gorm/compare/v0.3.0...v1.0.0
[0.3.0]: https://github.com/pamungkasandono/rsql-gorm/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/pamungkasandono/rsql-gorm/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/pamungkasandono/rsql-gorm/releases/tag/v0.1.0
