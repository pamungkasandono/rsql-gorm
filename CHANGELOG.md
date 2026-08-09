# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
