# Contributing to evrFileTools

## Versioning

This project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html) (semver).

- **MAJOR** (`vX.0.0`): Changes that break existing CLI flags, output formats, or
  Go package APIs that downstream code depends on.
- **MINOR** (`v0.X.0`): New features, commands, or modes that don't break existing
  usage.
- **PATCH** (`v0.0.X`): Bug fixes and documentation updates.

Releases are tagged as `vMAJOR.MINOR.PATCH` and published via GitHub Releases.

## Commit Messages

This project follows the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
specification.

### Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type | Description |
|------|-------------|
| `feat` | A new feature |
| `fix` | A bug fix |
| `docs` | Documentation only |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `test` | Adding or updating tests |
| `perf` | Performance improvement |
| `build` | Changes to the build system or dependencies |
| `ci` | Changes to CI configuration |
| `chore` | Other changes that don't modify source or test files |

### Scopes

Use the package or command name as the scope:

- `manifest`, `archive`, `hash`, `naming`, `audio`, `texture`, `tint`, `asset`
- `evrtools`, `texconv`, `showtints`, `symhash`

### Examples

```
feat(evrtools): add inventory mode for asset catalog
fix(manifest): use base-16 parsing for hex symbol filenames
test(manifest): add scanner round-trip tests
docs: update README with new CLI flags
build: upgrade Go to 1.24
refactor(naming): consolidate type symbol lookups
```

### Breaking Changes

Append `!` after the type/scope, or add a `BREAKING CHANGE:` footer:

```
feat(evrtools)!: rename -data flag to -datadir

BREAKING CHANGE: The -data flag has been renamed to -datadir.
Scripts using the old flag name will need to be updated.
```

## Development

### Prerequisites

- Go 1.24 or later
- libsquish (optional, for texconv PNG encoding)

### Build and Test

```bash
make build    # Build all CLI tools to bin/
make test     # Run all tests
make check    # Format, vet, and test
```

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Handle errors explicitly; don't ignore return values from `Close()` or `Flush()`
- Use table-driven tests with `t.Run()` subtests
- Prefer `io.Reader`/`io.Writer` interfaces over concrete types in function signatures

### Pull Requests

1. Create a feature branch from `main`
2. Make focused, minimal changes
3. Add tests for new functionality
4. Ensure `make check` passes
5. Use conventional commit messages
6. Address all review comments before requesting re-review
