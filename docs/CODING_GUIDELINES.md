# Alloy Coding Guidelines

This document outlines the coding standards and best practices for the Alloy project. Following these guidelines ensures code quality, maintainability, and consistency across the project.

## 1. Modern Go Best Practices

- **Formatting**: All code must be formatted using `gofmt` (or `goimports`).
- **Linting**: We use `golangci-lint` for static analysis. Ensure your code passes all linting rules before submitting.
- **Error Handling**: 
  - Use `fmt.Errorf("...: %w", err)` for error wrapping.
  - Check for specific errors using `errors.Is` and `errors.As`.
  - Avoid `panic()` in production code unless it's a truly unrecoverable state at startup.
- **Context**: Use `context.Context` for cancellation and timeouts, especially in IPC and plugin management.
- **Concurrency**: Use goroutines and channels carefully. Prefer communication over shared memory.
- **Logging**: 
  - All components must extensively log their behavior to facilitate debugging and monitoring.
  - Use structured logging (e.g., `slog` from the Go standard library).
  - Include relevant context (trace IDs, user IDs, component names) in log entries.
  - Use appropriate log levels (`DEBUG` for verbose info, `INFO` for general state changes, `WARN` for non-critical issues, `ERROR` for failures).

## 2. Unit Testing and Testability

- **Dependency Injection**: Use interfaces to define dependencies, allowing for easy mocking in tests.
- **Table-Driven Tests**: Use table-driven tests for complex logic to cover multiple cases efficiently.
- **Mocking**: Use tools like `moq` or manual implementations of interfaces for mocking.
- **Package Placement**: Keep tests in the same package as the code they test (`package_test.go`).
- **Coverage**: Aim for a minimum of 85% unit test coverage on core logic (`pkg/`).
- **Functional Testing**: 
  - The backend must include a functional test suite that verifies the interaction between a mock frontend, the kernel, and a mock plugin.
  - Functional tests should reside in the `tests/` directory or as integration tests in the `pkg/kernel` package.
  - These tests should verify end-to-end message flow: Frontend -> Kernel -> Plugin -> Kernel -> Frontend.

## 3. Automation and Tooling

### Justfile
We use `just` as a command runner. Common tasks include:
- `just fmt`: Format all Go files.
- `just lint`: Run `golangci-lint`.
- `just test`: Run all unit tests.
- `just build`: Compile the backend and CLI.

### CI/CD
GitHub Actions are used for continuous integration. Every PR must:
- Pass linting.
- Pass all unit tests.
- Successfully build all components.

## 4. WASM Plugin Development

- **Small Data**: Pass by value using the standard `api.Message` structure.
- **Large Data**: Use the Kernel's `RefID` system. Never pass multiple megabytes of data directly in a message payload.
- **Error Handling**: Plugins should return clear error codes that the Kernel can translate into `TypeResponse` error messages for frontends.
- **State**: Assume the WASM instance can be restarted at any time. Persist critical state via the scoped virtual filesystem or the Buffer Manager.
- **Responsiveness**: Avoid long-running synchronous loops. The guest should yield control back to the host regularly.

## 5. Documentation

- **Comments**: All exported functions, types, and constants must have a comment.
- **Internal**: Document complex internal logic to assist future maintainers.
- **Architecture**: Changes to the core architecture must be reflected in `docs/ARCHITECTURE.md`.
