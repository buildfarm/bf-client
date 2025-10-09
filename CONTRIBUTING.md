## Contributing

Thank you for helping improve `bf-client`! This guide outlines the basics for making high‑quality contributions that are easy to review and maintain.

### Prerequisites
- Go 1.23 (see `go.mod`)
- Bazel (build/test)
- Git

### Getting started
1. Fork the repo and create a feature branch from the default branch.
2. Make small, focused commits with clear messages.
3. Open a pull request early for feedback; keep PRs scoped and reviewable.

### Build and test
- Bazel (preferred):
  - Build: `bazel build //...`
  - Test: `bazel test //...`
- Native Go (if helpful locally):
  - `go build ./...`
  - `go test ./...`

All PRs should pass tests and build cleanly.

### Formatting and static checks
- Always run formatting before committing:
  - `go fmt ./...`
  - Optionally also: `gofmt -s -w .`
- Prefer to run basic static checks locally:
  - `go vet ./...`

CI may reject PRs that aren’t formatted. If you’re unsure, re‑run `go fmt ./...`.


### Pre-commit (optional but encouraged)
Use `pre-commit` to auto‑format and vet changes locally.

Then run:
```bash
pre-commit install
```

### Commit and PR checklist
- [ ] Code compiles and tests pass (`bazel test //...`)
- [ ] Public APIs and non‑obvious logic documented
- [ ] PR description explains the why and the what

### Communication
- Prefer small, iterative PRs; include context and tradeoffs.
- Be kind and constructive in reviews; propose changes with rationale.

Thanks for contributing!

