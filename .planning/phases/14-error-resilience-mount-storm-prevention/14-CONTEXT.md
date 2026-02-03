# Phase 14: Error Resilience and Mount Storm Prevention - Context

**Gathered:** 2026-02-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement protection mechanisms to prevent corrupted filesystems from causing cluster-wide failures and prevent the CSI driver from interfering with system volumes. This phase addresses two critical production risks discovered during Phase 13: mount storms (thousands of duplicate mounts from corrupted filesystems) and potential system volume disconnection (driver could disconnect `/var` mounts on diskless nodes, bricking the cluster).

The phase delivers defensive safeguards, not new volume functionality.

</domain>

<decisions>
## Implementation Decisions

### NQN Filtering Behavior

- **Configuration requirement:** Driver MUST refuse to start if `CSI_MANAGED_NQN_PREFIX` environment variable is not configured (no hard-coded fallback)
- **Prefix format:** Full NQN prefix including base (e.g., `nqn.2000-02.com.mikrotik:pvc-`), not just final segment
- **Validation:** Validate NQN prefix format at startup - fail fast if invalid
- **Non-CSI volume handling:** When encountering volumes with wrong NQN prefix (e.g., `nixos-*` system volumes), skip them and log at debug level
- **Default value (Helm):** `nqn.2000-02.com.mikrotik:pvc-` configurable via Helm value `nqnPrefix` and env var `CSI_MANAGED_NQN_PREFIX`

### Mount Storm Prevention

- **Detection locations:** Both during procmounts parsing AND before mount operations (defense in depth)
- **Threshold behavior:** Research appropriate threshold and behavior (100 entries mentioned in success criteria needs validation)
- **Timeout on procmounts parsing:** 10 seconds max (from success criteria)
- **Cleanup policy:** Prevent new duplicate mounts only - don't attempt to clean up existing duplicates (requires manual cleanup for safety)

### Circuit Breaker Policy

- **Triggers:** Both filesystem health check failures AND mount operation failures trigger circuit breaker
- **Backoff strategy:** Exponential backoff (longer block time each failure)
- **State storage:** In-memory only (lost on driver restart - provides fresh start)
- **Manual reset:** Support annotation-based reset on PV (standard Kubernetes pattern for operator intervention)

### Graceful Degradation

- **Startup health checks:** Refuse to start if:
  - NQN prefix not configured
  - NQN prefix validation fails
  - Required tools missing (exact tool list is Claude's discretion)
- **Filesystem health checks:** Run before every mount operation (not cached)

### Claude's Discretion

- Exact duplicate mount threshold (validate if 100 is appropriate)
- What to do when duplicate threshold exceeded (fail vs block+override vs warn)
- Timeout handling when procmounts parsing exceeds 10s
- Filesystem check tool selection and flags (fsck variants, per-filesystem-type checks)
- Which tools are required vs optional for startup validation
- Graceful shutdown behavior when operations still running after 30s
- Exponential backoff parameters (initial delay, max delay, multiplier)

</decisions>

<specifics>
## Specific Ideas

- **Production incident context:** Phase 13 discovered two critical risks:
  1. Corrupted filesystem caused mount storm (thousands of duplicate mounts)
  2. Orphan cleaner could disconnect system volumes (diskless nodes mount `/var` from RDS via NVMe-oF with NQN pattern `nixos-*`)

- **Critical protection:** Without NQN filtering, driver operations could brick nodes by disconnecting their root filesystem

- **Success criteria from roadmap:**
  1. NQN filtering prevents system volume disconnect
  2. Configurable NQN prefix via Helm and env var
  3. Procmounts parsing timeout (10s)
  4. Duplicate mount detection (max 100 entries per device)
  5. Graceful shutdown (30s even during stuck ops)
  6. Filesystem health check before NodeStageVolume
  7. Circuit breaker prevents retry storms

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 14-error-resilience-mount-storm-prevention*
*Context gathered: 2026-02-03*
