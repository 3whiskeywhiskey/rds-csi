---
created: 2026-02-06T19:40
title: Add test case 8 for snapshot operations to HARDWARE_VALIDATION.md
area: docs
files:
  - docs/HARDWARE_VALIDATION.md:80
---

## Problem

HARDWARE_VALIDATION.md has 7 test cases (TC-01 through TC-07) but no test case for snapshot operations. Operators validating RDS hardware before production deployment won't test snapshot functionality, which could lead to discovering snapshot issues in production.

Identified in v0.10.0 milestone audit as tech debt item #1. This breaks E2E user flow 3 (Hardware Validation → Snapshot Testing → Troubleshooting).

## Solution

Add test case 8 (TC-08) after line 80 with the following structure:

**Test Case 8: Volume Snapshot Operations**
- **Objective:** Validate Btrfs snapshot create/restore/delete via CSI driver
- **Prerequisites:** Existing PVC with test data written
- **Steps:**
  1. Create VolumeSnapshot from existing PVC
  2. Verify snapshot created via SSH: `/disk print detail where slot~"snap-"`
  3. Create new PVC from snapshot (restore)
  4. Verify restored volume contains original data
  5. Delete snapshot via kubectl
  6. Verify snapshot removed via SSH
- **Success Criteria:**
  - Snapshot created within 5-10 seconds
  - Restored volume data matches source
  - Snapshot deletion completes cleanly
- **Cleanup:** Delete test PVCs and VolumeSnapshots

**Effort:** 1-2 hours (write test case, validate against real RDS)
**Priority:** High (enables production validation workflow)
