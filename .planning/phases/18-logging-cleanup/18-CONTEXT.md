# Phase 18: Logging Cleanup - Context

**Gathered:** 2026-02-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Systematic reduction of production log noise through rationalized verbosity levels. Move verbose diagnostic output (RouterOS command responses, detailed state) from info level to debug level. Consolidate repetitive logging patterns. This is a cleanup phase - no new logging features, just moving existing logs to appropriate levels.

</domain>

<decisions>
## Implementation Decisions

### Primary Goal
- Eliminate unnecessary debug logging at info level
- Move verbose output (RouterOS command responses, detailed operation traces) to debug level
- Production logs (info level) should contain only actionable information: operation success/failure, errors that require attention

### RouterOS Command Output
- Command responses and detailed RouterOS output currently logged at info level → move to debug
- Info level: "Created volume pvc-xyz" (outcome only)
- Debug level: Full RouterOS CLI output, command syntax, parsing details
- Apply consistently across all RDS package operations (CreateVolume, DeleteVolume, etc.)

### Security Logger Consolidation
- Current: 300+ lines of repetitive operation logging
- Target: <50 lines using configurable helper
- Focus on consolidating patterns, not changing what gets logged
- Maintain security audit trail at appropriate verbosity level

### DeleteVolume Operation
- Current: 4-6 log statements per operation
- Target: Maximum 2 log statements (start + outcome or just outcome)
- Move intermediate steps to debug level
- Example: "Deleted volume pvc-xyz" (info) vs "Running /disk remove..." (debug)

### CSI Operation Verbosity
- Info level: Operation boundaries (start/complete), final outcomes, errors requiring action
- Debug level: Intermediate steps, parameter validation, state checks, command details
- Apply across all CSI operations (NodeStageVolume, NodePublishVolume, etc.)

### Claude's Discretion
- Exact helper function design for security logger
- Whether to use table-driven or function-based severity mapping
- Specific log message wording (as long as it's concise and actionable)
- Order of refactoring (which package/file to start with)

</decisions>

<specifics>
## Specific Ideas

**User's directive:**
"Remove all the unnecessary debug logging (or move it to debug level at least), such as how it logs the routeros output for every command"

**Key example:**
- BAD (current): Info-level logging of full RouterOS command output on every disk operation
- GOOD (target): Debug-level logging of command details, info-level only shows outcome

**Success indicators:**
- Production logs are quiet - no command output spam
- Can still troubleshoot by enabling debug level when needed
- Info level tells you WHAT happened, debug level tells you HOW

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope

</deferred>

---

*Phase: 18-logging-cleanup*
*Context gathered: 2026-02-04*
