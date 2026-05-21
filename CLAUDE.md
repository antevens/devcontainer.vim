# devcontainer.vim Development Guide

## Build Commands
- Build for current platform: `make build`
- Build for all platforms: `make build-all`
- Code formatting: `make fmt`
- Run linter: `make lint`

## Test Commands
- Run all tests: `make test`
- Run a single test: `go test -v ./[package] -run [test_name]`
- Example: `go test -v ./devcontainer -run TestStart`

## Code Style Guidelines
- Use standard Go format (`go fmt`)
- Follow staticcheck rules (ST1003, ST1016)
- Import order: standard library, third-party, project-specific
- Error handling: immediate checks and messages including context
- Use `fmt.Fprintf(os.Stderr, ...)` for errors, normal output to stdout
- Test resources: place in `/test/resource/` or `/test/project/`
- Use defer to ensure resource cleanup
- Use interface-based design for testability
- Platform-specific code: runtime checks and separate files as needed
