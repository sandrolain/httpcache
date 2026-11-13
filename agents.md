# Agents

Instructions for AI coding agents working on this repository.

## Project overview

- Go module: `github.com/sandrolain/httpcache` — a mostly RFC 9111 compliant HTTP cache (`http.RoundTripper`) with pluggable backends and wrappers.
- Go version is managed with `asdf` (see `.tool-versions`) and pinned in `go.mod`.
- Common tasks are defined in `taskfile.yaml` and run with `task <name>`.

## Common commands

- `task test` — run tests with race-friendly coverage profile (excludes `examples/`).
- `task test-integration` — run integration tests (`-tags=integration`); requires Docker (testcontainers).
- `task lint` — run golangci-lint.
- `task vet` — run go vet.
- `task fmt` — format with gofmt (`task fmt-check` to only check).
- `govulncheck ./...` — vulnerability scan (see `.github/workflows/security.yml`).

## Context

This is a fork of "github.com/gregjones/httpcache", a project archived in 2023 and no longer maintained. The goal is to keep updating it while maintaining backward compatibility with existing code that uses it, to add new features and fix bugs.

## Instructions

1. **Dependency updates**: Update dependencies in the `go.mod` file to use the latest versions of libraries, maintaining compatibility with existing code.
2. **Test writing**: Add tests for new features and ensure existing tests are updated and working.
3. **Documentation**: Update documentation to reflect changes made and provide clear instructions on how to use the updated package.
4. **Backward compatibility**: Ensure that any changes made do not break existing functionality. If breaking changes are necessary, document them clearly.
5. **Code quality**: Follow Go best practices for code quality, including proper error handling, code organization, and commenting.
6. **Performance**: Optimize the code for performance where possible without sacrificing readability or maintainability.
7. **Security**: Review the code for potential security vulnerabilities and address any issues found.
8. **Versioning**: Follow semantic versioning principles when making changes.
9. **AI**: Do not cite or mention AI tools in the code, comments, or documentation.

## Approach

- Analyze the existing codebase to understand its structure and functionality.
- Identify areas that require updates or improvements based on the instructions provided.
- Be critical and suggest changes that enhance the overall quality of the code while adhering to the instructions.

## Context directory

- Track changes in markdown files into the file `context/changes/{YYYY-MM-DD}.md` with a short description of the change and the date of the change.
- Use a file to preserve context and description of the project into `context/project.md`, creating it if it does not exist.
- Use the `/context` directory to store additional context files if needed.

**IMPORTANT**: Do not refer to files in the `/context` directory in README files or documentation in the `/docs` directory. The context directory is internal only.

## Code and language

The code and the texts in the code and comments must be written in English.

## Commit messages

For commits use conventional commits format.

## After every change or integration

- run the specific tests to ensure the code works as expected and a minimum coverage of 80% is reached.
- assure that inline docs are updated and accurate.
- run `task lint` (or `golangci-lint run ./...`) and `govulncheck ./...` to ensure no issues are present.
- update the `TODO.md` file, if present, marking completed tasks.

## Tests

- Unit tests live next to the code in `<file>_test.go` files.
- Benchmark tests must be created in a separated file respecting the unit tests file, and with the suffix `_bench_test.go`.
- Integration tests go in `<file>_integration_test.go` files with the `//go:build integration` tag, and are only run with `task test-integration` or `go test -tags=integration`.

## GIT

Do not perform git operations, only suggest code changes.

## Examples

For the examples, use the `examples/` directory.
Do not create a go.mod file in the examples directories, but use the main project's go.mod file for dependencies.
