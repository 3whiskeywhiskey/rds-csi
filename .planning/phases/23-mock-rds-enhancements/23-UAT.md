---
status: testing
phase: 23-mock-rds-enhancements
source: 23-01-SUMMARY.md, 23-02-SUMMARY.md
started: 2026-02-04T23:22:00Z
updated: 2026-02-04T23:22:00Z
---

## Current Test

number: 1
name: Environment-based configuration without code changes
expected: |
  Mock RDS server can be configured via environment variables (MOCK_RDS_*) without modifying code.
  Setting MOCK_RDS_REALISTIC_TIMING=true enables timing simulation, MOCK_RDS_ERROR_MODE=disk_full triggers disk full errors.
awaiting: user response

## Tests

### 1. Environment-based configuration without code changes
expected: Mock RDS server can be configured via environment variables (MOCK_RDS_*) without modifying code. Setting MOCK_RDS_REALISTIC_TIMING=true enables timing simulation, MOCK_RDS_ERROR_MODE=disk_full triggers disk full errors.
result: [pending]

### 2. Realistic SSH latency simulation
expected: When MOCK_RDS_REALISTIC_TIMING=true, SSH operations have 150-250ms latency (200ms base + 50ms jitter) exposing timeout bugs.
result: [pending]

### 3. Error injection for disk full scenarios
expected: Setting MOCK_RDS_ERROR_MODE=disk_full causes CreateVolume to fail with "not enough space" error, driver retries correctly.
result: [pending]

### 4. Error injection for SSH timeout scenarios
expected: Setting MOCK_RDS_ERROR_MODE=ssh_timeout causes SSH operations to timeout, driver handles timeout errors correctly.
result: [pending]

### 5. Error injection for command failures
expected: Setting MOCK_RDS_ERROR_MODE=command_fail causes RouterOS commands to fail with parsing errors, driver handles command failures correctly.
result: [pending]

### 6. Stateful volume tracking across operations
expected: Sequential operations on same volume ID (create, query same ID, delete, delete same ID) behave correctly with idempotency validation.
result: [pending]

### 7. Concurrent SSH connections without corruption
expected: 50 parallel volume operations (10 goroutines × 5 operations) complete successfully without state corruption or data races.
result: [pending]

### 8. RouterOS-formatted output matching production
expected: Mock server returns RouterOS 7.16 formatted output that driver's parser validates successfully.
result: [pending]

### 9. Fast tests by default
expected: Without MOCK_RDS_REALISTIC_TIMING=true, tests run fast with no timing delays. Existing sanity tests pass without environment variables.
result: [pending]

### 10. Command history tracking with concurrent operations
expected: Mock server maintains accurate command history even during concurrent operations, useful for debugging test failures.
result: [pending]

## Summary

total: 10
passed: 0
issues: 0
pending: 10
skipped: 0

## Gaps

[none yet]
