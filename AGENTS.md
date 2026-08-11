# AGENTS.md

RSQL-style filter builder for GORM. Single Go package `rsql` at the repo root (module `github.com/pamungkasandono/rsql-gorm`), no subpackages.

## Commands

- Test: `go test ./...`
- Vet: `go vet ./...`
- CI gate (`.github/workflows/ci.yml`): `go vet ./...` then `go test -race -v ./...` on Go 1.26.x. No linter config exists; `go vet` is the only static check.
- Requires Go 1.26.3+ (`go.mod`). GORM v1 only.

## Testing quirks

- Fully offline, no external database. Most tests use GORM `DryRun` mode; `sql_injection_test.go` additionally opens a real in-memory SQLite (`file::memory:?cache=shared`) via the pure-Go `github.com/glebarez/sqlite` driver (no CGO).
- Keep tests offline: DryRun assertions verify generated SQL strings; use the in-memory SQLite only for semantics cases.

## Architecture (parse → validate → build)

- `parser.go`: RSQL string → AST (`Node`/`AndNode`/`OrNode`/`ComparisonNode` in `ast.go`). Enforces DoS bounds: filter length 8192 bytes, paren depth 100, `=in=`/`=out=` list size 2000.
- `validator.go`: validates selectors against the model, enforces `defaultMaxJoinDepth` = 5.
- `query_builder.go`: builds joins + WHERE clauses; also all operator/wildcard logic. `rootInfoCache` (`sync.Map`) caches root model metadata per `reflect.Type`.
- `apply.go`: `ApplySort`, `ApplyPagination`, `BuildPageableQuery`/`PagedQuery`, `BuildQueryWithParams`.
- `params.go` / `list_params.go`: pagination (`DefaultLimit` 10, `MaxLimit` 1000, `MaxPage` 10000) and HTTP-param parsing.
- `alias.go`: `WithAliases` stores public→internal field-path maps in the GORM session via `db.Set(aliasKey(rootType), ...)`; scoped per `*gorm.DB` instance, not global.

## Conventions & gotchas

- **Selectors are Go struct field names** (matched case-insensitively via `strings.EqualFold`), never table/column names. DB columns come from `gorm:"column:..."` tags, snake_case fallback. Single-segment selectors are also validated against the root model; unknown fields error instead of reaching SQL.
- **Models must implement `TableName()`**; checked eagerly in `newQueryBuilder` and `WithAliases`.
- Values are always parameterized (`?` placeholders) and selectors are validated; never interpolate a raw selector or value into SQL.
- Wildcard semantics (`query_builder.go`): `*` → `%`; `\*` is a literal asterisk; `%`, `_`, `\` are always literal. Patterns use `ILIKE ... ESCAPE '\'` (explicit escape for dialect consistency).
- `==null`/`!=null` (case-insensitive) → `IS NULL`/`IS NOT NULL`; inside `=in=`/`=out=` `null` stays a literal string.
- `!=` and `=out=` on a **has-many** relation generate `NOT IN (SELECT ...)` subqueries, not joins, so roots without matching children are correct.
- `BuildQuery` applies no LIMIT. Request-facing endpoints should use `BuildPageableQuery`/`BuildQueryWithParams`.
- **GORM `Count` pollutes the statement**, so `BuildPageableQuery` snapshots clauses/joins; call `pq.NewQuery()` once for `Count` and again for `Find`; never reuse the raw statement across both.
- Aliases use longest dot-prefix match; a leaf path like `brandLabel → Brand.Name` is allowed (rename). Alias values are validated eagerly against the model.
- No emdashes anywhere in this project: not in comments, docs, or code. The codebase was deliberately cleaned of them (commit `6c86286`). Use hyphens.
- Keep the changelog updated for user-facing changes: `CHANGELOG.md` follows Keep a Changelog + SemVer, releases landed via `chore(release): prepare vX.Y.Z` commits with `[Unreleased]` compare links.

## API surface (do not break)

`Parse`, `BuildQuery`, `ApplySort`, `ApplyPagination`, `BuildQueryWithParams`, `BuildPageableQuery`, `ParseSort`, `ParseListParams`, `WithAliases`, `Aliases`, `Params`, `PagedQuery`, `Node` types, and `DefaultLimit`/`MaxLimit`/`MaxPage` are public and referenced in the README API table.
