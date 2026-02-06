# Phase 23: Mock RDS Enhancements - Context

**Gathered:** 2026-02-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Mock RDS server that matches real hardware behavior to enable reliable CI testing. The mock handles SSH connections, RouterOS CLI commands, and simulates realistic timing, errors, and state management for volume lifecycle operations. This enables automated sanity tests and E2E tests to run without real hardware.

</domain>

<decisions>
## Implementation Decisions

### Mock Behavior Realism
- Fixed 200ms SSH latency to catch timeout issues consistently
- Support RouterOS version parameter (tests can specify version like 7.1, 7.16) to validate compatibility across versions
- Strict command parsing — reject malformed commands to expose driver bugs in command construction
- Realistic operation delays controlled by environment variable (fast by default, set MOCK_RDS_REALISTIC_TIMING=true for timing validation)
- Document RouterOS quirks from real RDS output, Claude decides which quirks matter for testing

### Error Injection Design
- Annotation-based error injection via test framework integration
- Fine-grained control — can fail specific steps (e.g., "SSH connect succeeds but command execution fails")
- Test suite level scoping — error config applies to entire suite, not per-test isolation
- Must-have error scenarios:
  - Disk full scenarios (simulate "not enough space" errors)
  - SSH connection failures (timeout, connection refused, authentication failure)
  - Command parsing errors (malformed output, missing fields, unexpected format)
  - Concurrent operation conflicts (race conditions on same volume)

### State Management
- In-memory state only (resets on mock restart, fast for unit tests)
- Full concurrency with locking — allow concurrent SSH connections, use mutex to protect state
- Match real RDS behavior exactly for duplicate volumes (same error message as production RDS)
- Configurable history depth — tests can enable operation history tracking when needed, disabled by default for performance

### Test Observability
- Structured logs (JSON) with operation details, timestamps, parameters
- Log error injection configuration at test start AND tag affected operations
- Logs are sufficient for debugging (no special dump mechanism needed)

### Claude's Discretion
- Log verbosity levels — choose appropriate logging strategy (standard debug/info/warn/error vs custom test levels)
- Specific operation timing values (exact delays for disk add, disk remove when realistic timing enabled)
- Which RouterOS quirks to simulate from documented list

</decisions>

<specifics>
## Specific Ideas

- "We want to avoid bloating CI times — fast by default, realistic timing only when explicitly testing for it"
- "I don't know exact operation timing from real hardware — Claude should determine realistic delays based on observations or defaults"

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 23-mock-rds-enhancements*
*Context gathered: 2026-02-04*
