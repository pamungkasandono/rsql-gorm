# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `ApplySort` to apply validated `ORDER BY` clauses, including nested selectors
  (e.g. `Role.Name`) that resolve through the existing join machinery.
- `ApplyPagination` to apply clamped `LIMIT`/`OFFSET` from a `Pagination`.
- `BuildQueryWithParams` to apply filter, sort and pagination in a single call.

### Changed

- `resolveSelector` now rejects selectors whose last segment is not a known
  struct field, instead of silently falling back to the raw name. Query
  builder and sort fields must reference Go struct field names; DB column
  names are still derived from `gorm` tags.
- `BuildQuery` extracted its model initialization into `newQueryBuilder`, now
  shared with `ApplySort`.

### Fixed

- `ParseListParams` now rejects non-numeric page and page size values with an
  error, instead of silently clamping them to the defaults.

### Added

- `==null` / `!=null` (case-insensitive) now map to `IS NULL` / `IS NOT NULL`
  instead of comparing the literal string. Wildcard values (e.g. `*null*`)
  still generate `ILIKE` searches; `null` inside `=in=` / `=out=` remains a
  string literal.
- `\*` escapes the wildcard so a literal asterisk can be searched: `a\*b`
  becomes the pattern `a*b` instead of `a%b`. An unescaped `*` still maps to
  SQL `%`.

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

[Unreleased]: https://github.com/pamungkasandono/rsql-gorm/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/pamungkasandono/rsql-gorm/releases/tag/v0.1.0
