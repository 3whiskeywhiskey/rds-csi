---
created: 2026-02-06T19:40
title: Create E2E snapshot tests for real RDS hardware validation
area: testing
files:
  - test/e2e/snapshot_test.go
---

## Problem

Snapshot feature is only validated via CSI sanity tests (which use mock RDS), not against real RDS hardware. This means Btrfs snapshot operations (/disk add type=snapshot, snapshot-of, etc.) aren't tested in E2E suite.

Identified in v0.10.0 milestone audit as tech debt item #3. Missing integration connection from Phase 26 to test/e2e/.

**Impact:** Medium severity - CSI sanity tests provide spec compliance validation, but not actual Btrfs operation validation.

## Solution

Create test/e2e/snapshot_test.go with Ginkgo test suite covering:

1. **Basic snapshot lifecycle:**
   - Create PVC → Write data → Create snapshot → Verify snapshot exists
   - Delete snapshot → Verify snapshot removed

2. **Restore from snapshot (same size):**
   - Create PVC from snapshot → Verify data matches original

3. **Restore from snapshot (larger size):**
   - Create larger PVC from snapshot → Verify data matches, extra space available

4. **Snapshot idempotency:**
   - Create snapshot with same name twice → Should succeed both times

5. **ListSnapshots pagination:**
   - Create 10+ snapshots → List with max_entries=5 → Verify pagination

**Effort:** 4-6 hours (write tests, validate against real RDS, handle timing issues)
**Priority:** Nice-to-have (deferred based on production feedback)

**Note:** This is optional cleanup work. Consider deferring to v0.11.0 or later based on whether production users report snapshot issues.
