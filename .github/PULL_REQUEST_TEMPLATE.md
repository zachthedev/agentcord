## Summary

<!-- Brief description of changes. Link related issues with "Closes #123" -->

## Type of Change

- [ ] Bug fix
- [ ] New feature
- [ ] New tool integration
- [ ] Refactor / code improvement
- [ ] Documentation
- [ ] CI / build

## Affected Components

- [ ] Daemon (Go)
- [ ] Hook scripts (bash/PowerShell)
- [ ] Dispatch (TypeScript)
- [ ] Config / codegen
- [ ] Installer
- [ ] Assets
- [ ] CI / workflows

## Client Tools Affected

- [ ] Claude Code
- [ ] Cursor
- [ ] Windsurf
- [ ] All clients
- [ ] N/A

## Testing

- [ ] `make test` (Go tests pass)
- [ ] `make test-sh` (BATS shell tests pass)
- [ ] `make test-ps1` (Pester PowerShell tests pass)
- [ ] `make test-ts` (dispatch tests pass)
- [ ] `make lint` (no lint errors)
- [ ] `go generate ./...` produces no diff

## Platforms Tested

- [ ] macOS
- [ ] Linux
- [ ] Windows
- [ ] WSL

## Checklist

- [ ] Code follows project conventions (gofumpt, error wrapping, slog)
- [ ] Generated files are up to date (`go generate ./...`)
- [ ] Config docs updated if config fields changed
- [ ] Tests added/updated for new behavior
- [ ] README updated if user-facing behavior changed
