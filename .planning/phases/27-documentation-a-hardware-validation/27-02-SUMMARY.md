---
phase: 27
plan: 02
type: summary
subsystem: documentation
tags: [documentation, capabilities, csi-spec, comparison, known-limitations]
completed: 2026-02-05

# Dependencies
requires:
  - phase: 25.2
    provides: Production-ready driver with comprehensive resilience features
  - phase: 22-25
    provides: Test coverage and validation infrastructure

provides:
  - Honest CSI capability gap analysis with peer driver comparison
  - Known limitations documentation with version-specific constraints
  - Feature comparison matrix (RDS CSI vs AWS EBS CSI vs Longhorn)
  - Operator-facing deployment decision guidance

affects:
  - phase: 28
    impact: Helm chart documentation can reference CAPABILITIES.md
  - future-operator-evaluations
    impact: Clear expectations before deployment

# Technical Context
tech-stack:
  added: []
  patterns:
    - CSI capability gap analysis framework
    - Known limitations with detection/workaround structure
    - Honest "why not" explanations for missing features

key-files:
  created:
    - docs/CAPABILITIES.md
  modified:
    - README.md

decisions:
  - id: DOC-02-1
    what: Use CSI spec coverage tables (Identity, Controller, Node services)
    why: Operators evaluating drivers need spec-level comparison
    alternatives: Free-form feature list (less structured)
    impact: Clear mapping between CSI spec and implementation

  - id: DOC-02-2
    what: Compare against AWS EBS CSI and Longhorn (not iSCSI or Ceph)
    why: AWS EBS CSI is cloud-native reference, Longhorn is on-prem Kubernetes-native
    alternatives: Compare against SPDK CSI or iSCSI drivers (too niche)
    impact: Operators see apples-to-apples comparison with familiar drivers

  - id: DOC-02-3
    what: Acknowledge single-server architecture vs distributed storage upfront
    why: Prevents unfair HA/replication comparisons; sets correct expectations
    alternatives: Compete on features (defensive, misleading)
    impact: Honest positioning in target niche (homelab, small clusters)

  - id: DOC-02-4
    what: Provide "why not" explanation for every missing feature
    why: Transparency builds trust; architectural constraints vs planned features
    alternatives: Just list supported/not supported (leaves questions)
    impact: Operators understand limitations are intentional, not oversights

  - id: DOC-02-5
    what: Known limitations include detection methods and workarounds
    why: Operators need actionable info to diagnose and mitigate limitations
    alternatives: Just list constraints (not helpful)
    impact: Self-service troubleshooting; reduces support burden

metrics:
  duration: 3 minutes
  tasks: 2
  commits: 2
  lines-added: 397
  lines-removed: 0
---

# Phase 27 Plan 02: CSI Capability Gap Analysis & Known Limitations

**One-liner:** Transparent CSI capability comparison with AWS EBS CSI/Longhorn and structured known limitations documentation

## What Was Built

Created comprehensive capability gap analysis documentation comparing RDS CSI Driver against mature peers (AWS EBS CSI, Longhorn), with honest assessment of what's supported, what's missing, and why. Added structured Known Limitations section to README.md with version-specific constraints, detection methods, and workarounds.

### Key Deliverables

**1. docs/CAPABILITIES.md (357 lines)**
- CSI specification coverage tables for Identity, Controller, and Node services
- Feature comparison matrix: RDS CSI vs AWS EBS CSI vs Longhorn
- 20+ features compared across provisioning, access modes, reliability, performance
- Unique advantages section (NVMe/TCP latency, file-backed volumes, attachment reconciliation)
- Architectural differences explanation (single-server vs distributed storage)
- "What's Not Supported and Why" section with honest explanations
  - Volume cloning: RouterOS doesn't expose Btrfs reflink via CLI
  - ReadWriteMany: NVMe/TCP single-initiator protocol limitation
  - Controller HA: Single storage server makes controller HA redundant
  - Volume encryption: RouterOS-level limitation
- Roadmap with Phase 26 snapshot plans

**2. README.md Known Limitations Section**
- RouterOS version compatibility (7.1+ required, detection via SSH errors)
- NVMe device timing assumptions (30s timeout, workaround: check network latency)
- Dual-IP architecture recommendations (single-IP works but impacts performance)
- Single controller instance (10s provisioning unavailability during restarts)
- Access mode restrictions (RWO only, no multi-attach)
- Volume size minimum (1 GiB, sub-1GiB rounded up)
- Link to CAPABILITIES.md for comprehensive comparison

**3. Documentation Cross-references**
- README.md links to CAPABILITIES.md in Documentation section
- CAPABILITIES.md references TESTING.md for tested vs untested capabilities
- Known Limitations section points to CAPABILITIES.md for full driver comparison

## Tasks Completed

| Task | Name | Commit | Files | Status |
|------|------|--------|-------|--------|
| 1 | Create CAPABILITIES.md with feature comparison matrix | 9f4cf6f | docs/CAPABILITIES.md | ✅ Complete |
| 2 | Add Known Limitations section to README.md | db047fc | README.md | ✅ Complete |

## Decisions Made

**DOC-02-1: Use CSI Spec Coverage Tables**
- Structured tables for Identity, Controller, Node services
- Maps CSI capabilities to implementation status (supported/planned/not planned)
- Provides spec-level comparison that operators evaluating drivers need

**DOC-02-2: Compare Against AWS EBS CSI and Longhorn**
- AWS EBS CSI: Cloud-native reference implementation
- Longhorn: Kubernetes-native distributed storage
- Avoids niche comparisons (SPDK CSI, iSCSI drivers)
- Gives operators familiar reference points

**DOC-02-3: Acknowledge Single-Server Architecture Upfront**
- "Different architectural goals, not competing with distributed storage"
- Explains reliability model difference (hardware vs replication)
- Prevents unfair HA/replication feature comparisons
- Positions driver honestly in target niche (homelab, small clusters)

**DOC-02-4: "Why Not" for Every Missing Feature**
- Volume cloning: RouterOS CLI constraint
- Multi-attach: NVMe/TCP protocol limitation
- Controller HA: Single storage server makes it redundant
- Encryption: RouterOS-level implementation needed
- Builds trust through transparency

**DOC-02-5: Actionable Known Limitations**
- Each limitation has: requires/impact/detection/workaround
- Detection methods enable self-service troubleshooting
- Workarounds provide migration paths or alternatives
- Version-specific (RouterOS 7.1+, 30s timeout, 1GiB minimum)

## Technical Highlights

**Honest Gap Assessment**
- Every "Not Planned" or "Not Supported" capability has explanation
- Distinguishes architectural constraints from planned features
- No defensive marketing; focuses on fit-for-purpose positioning

**Feature Comparison Matrix**
- 30+ features compared across 3 drivers
- Legend: ✅ Supported, 🔄 Planned, ⚠️ Limited, ❌ Not supported
- Covers provisioning, access modes, topology, reliability, performance, monitoring

**Architectural Context**
- Single-server vs distributed storage reliability models
- NVMe/TCP vs iSCSI vs cloud API protocol differences
- SSH CLI vs REST API vs cloud SDK management approaches
- When to choose RDS CSI vs alternatives (clear use case guidance)

**Unique Advantages Documented**
- NVMe/TCP: ~1ms latency vs ~3ms iSCSI (quantified benefit)
- File-backed volumes: Thin provisioning on Btrfs RAID
- SSH management: Auditable, debuggable, human-readable
- Attachment reconciliation: Born from production incident (Phase 25.1)
- NVMe-oF reconnection: Survived production RDS crashes
- KubeVirt: Live migration validated (~15s window)

**Known Limitations Structure**
- Version-specific requirements (RouterOS 7.1+, kernel 5.0+)
- Detection methods (log patterns, kubectl output)
- Workarounds (network checks, configuration alternatives)
- Links to full capability analysis

## Test Results

No automated tests for documentation content. Manual validation:

✅ Line count: 357 lines (exceeds 200 line minimum)
✅ AWS EBS mentions: 6 (adequate comparison coverage)
✅ Longhorn mentions: 12 (comprehensive comparison)
✅ Gap assessment: 10+ "Not Planned/Not Supported" with explanations
✅ "Why not" explanations: Present for every missing feature
✅ Known Limitations: 6 specific limitations with detection/workaround
✅ Cross-references: README → CAPABILITIES.md, CAPABILITIES → TESTING.md

## Deviations from Plan

None - plan executed exactly as written.

## Lessons Learned

**Documentation Positioning Strategy**
- Honest gap analysis builds more trust than feature marketing
- Explaining "why not" prevents perception of oversight
- Architectural differences section frames comparison fairly
- "Choose X when..." guidance helps operators self-select

**Operator-Focused Content**
- Detection methods enable self-service troubleshooting
- Workarounds provide migration paths
- Version-specific constraints guide deployment decisions
- Feature comparison matrix enables quick evaluation

**Unique Advantages as Differentiator**
- NVMe/TCP latency (~1ms) vs iSCSI (~3ms) is quantifiable
- Production-tested resilience (Phase 25.1 attachment reconciliation)
- KubeVirt validation (live migration) addresses specific use case
- SSH management is feature, not limitation (auditability)

## Next Phase Readiness

**Phase 27-03: Update Testing Documentation**
- Can reference CAPABILITIES.md for tested vs untested capabilities
- Known limitations provide context for test scope decisions
- Feature comparison helps explain E2E test coverage priorities

**Phase 28: Helm Chart**
- Chart documentation can link to CAPABILITIES.md for feature overview
- Known limitations inform default values (30s timeout, single controller)
- Architectural differences section guides values.yaml structure

**Future Quick Tasks**
- Update CAPABILITIES.md when Phase 26 snapshots ship
- Add new known limitations as discovered in production
- Expand feature comparison if new CSI drivers emerge

## Files Modified

```
docs/CAPABILITIES.md          +357 (created)
README.md                      +40 (Known Limitations section added)
```

## Related Context

- **27-RESEARCH.md Pattern 3:** Capability Gap Analysis Framework
- **27-RESEARCH.md Pattern 4:** Known Limitations Documentation
- **Phase 25.1 Accomplishments:** Attachment reconciliation (unique advantage)
- **Phase 25.2 Accomplishments:** Production-readiness features documented
- **TESTING.md:** CSI capability matrix (cross-referenced)

## Metrics

- **Duration:** 3 minutes (execution time)
- **Tasks:** 2/2 completed
- **Commits:** 2 (one per task, atomic)
- **Lines added:** 397 (357 CAPABILITIES.md + 40 README.md)
- **Documentation cross-references:** 3 (README→CAPABILITIES, CAPABILITIES→TESTING, Known Limitations→CAPABILITIES)

## Conclusion

Successfully created transparent capability gap analysis and structured known limitations documentation. Operators evaluating RDS CSI can now quickly understand what it supports, what it doesn't, and why. The gap analysis is honest and frames architectural differences fairly (single-server vs distributed). Known limitations are specific enough (with version numbers, detection methods, workarounds) to prevent deployment surprises.

The documentation positions RDS CSI honestly in its target niche (homelab, small clusters with MikroTik hardware) rather than competing with distributed storage solutions on features where architectural differences make comparison unfair. The "why not" explanations for missing features build trust through transparency.

Next: Phase 27-03 will update testing documentation to reflect current state (v0.9.0 accomplishments, E2E test infrastructure, CI coverage enforcement).
