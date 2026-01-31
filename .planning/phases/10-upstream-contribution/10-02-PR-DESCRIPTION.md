# KubeVirt PR Description

**What this PR does / why we need it:**

Fixes a race condition in volume hotplug where concurrent hotplugging of
multiple volumes causes the virt-controller to delete old attachment pods
before new volumes reach the Ready phase, resulting in VM pause and I/O errors.

**Before:** `cleanupAttachmentPods()` deleted old pods immediately when new
pods existed, regardless of volume readiness state. This caused the block device
for existing volumes to disappear from the VM when a new volume was hotplugged.

**After:** `cleanupAttachmentPods()` waits until all hotplug volumes reach
`VolumeReady` phase before deleting old attachment pods.

**The fix:**
- Add `allHotplugVolumesReady()` helper that checks if all hotplug volumes have
  reached VolumeReady phase in VMI status
- Modify `cleanupAttachmentPods()` to early return if volumes not ready
- Guard conditions ensure volume removal (numReadyVolumes=0) is unaffected

**Technical details:**

The bug occurs in `pkg/virt-controller/watch/vmi/volume-hotplug.go`:

1. When a new volume is hotplugged, the old attachment pod (serving existing volumes)
   doesn't match the new ready volume set (existing + new volumes)
2. Old pod is classified as "old" and immediately deleted
3. New attachment pod is created but hasn't reached VolumeReady phase yet
4. Block devices for existing volumes disappear from VM during the transition
5. VM filesystem becomes inaccessible until new pod becomes ready

The existing `volumeReadyForPodDelete()` check protects the removal scenario
(ensures volumes have completed unmounting), but doesn't protect the add scenario
(doesn't check if new pod's volumes are ready before deleting old pod).

This fix adds a check that ALL hotplug volumes in `vmi.Status.VolumeStatus` have
`Phase == VolumeReady` before allowing old pod deletion, ensuring:
- The new pod is running
- virt-launcher has mounted the volumes
- Block devices are available to the VM

**Validation:**

Validated on production-like cluster with nested K3s worker VM running on KubeVirt:

✅ **Multi-volume hotplug:** Hotplugged two volumes concurrently - VM stayed Running,
   no I/O errors, existing volumes remained accessible throughout operation

✅ **Volume removal:** Removed hotplugged volumes - clean detachment, no errors

✅ **Regression check:** Single-volume hotplug functionality remains intact

Before the fix: Concurrent hotplug caused VM pause and I/O errors as old attachment
pods were deleted prematurely.

After the fix: `allHotplugVolumesReady()` check prevents premature cleanup, old
attachment pods remain until all volumes reach VolumeReady phase.

**Which issue(s) this PR fixes:**

Fixes https://github.com/kubevirt/kubevirt/issues/6564
Fixes https://github.com/kubevirt/kubevirt/issues/9708
Related to https://github.com/kubevirt/kubevirt/issues/16520

**Special notes for your reviewer:**

- This is a first-time contribution from this account
- Validated on production-like cluster with nested K3s worker VM
- Multi-volume concurrent hotplug confirmed working without VM pause or I/O errors
- Volume removal path tested and confirmed unaffected
- Unit tests cover: bug reproduction, normal operation, single-volume regression, multi-volume edge case

**Release note:**
```release-note
Fix race condition in volume hotplug that caused VM pause and I/O errors
when hotplugging multiple volumes concurrently
```
