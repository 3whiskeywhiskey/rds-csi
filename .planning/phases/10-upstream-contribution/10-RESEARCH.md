# Phase 10: Upstream Contribution - Research

**Researched:** 2026-01-31
**Domain:** Open source contribution to kubevirt/kubevirt
**Confidence:** HIGH

## Summary

KubeVirt follows a standard GitHub-based contribution workflow with specific requirements around DCO sign-off, automated testing via Prow CI, and a two-phase review process using OWNERS files. Bug fixes like the hotplug race condition fix fall into the "simple changes" category that don't require a VEP (Virtualization Enhancement Proposal) and can proceed directly to PR submission.

The project emphasizes testability ("Untested features do not exist"), requires all commits to be DCO-signed, and uses automated CI testing that triggers after organization members approve new contributors with `/ok-to-test`. The average PR lifespan is 3 days, indicating an efficient review process. First-time contributors should expect initial PRs to need manual approval before CI runs.

**Primary recommendation:** Submit a well-structured PR with comprehensive unit tests, clear commit messages with DCO sign-off, concise release notes, and references to related issues. Keep the PR focused and small (200-400 LOC) for faster review. Engage with maintainers via the PR or #kubevirt-dev Slack channel if blocked.

## Standard Stack

### Core Tools and Infrastructure

| Tool/System | Version/Status | Purpose | Why Standard |
|-------------|----------------|---------|--------------|
| **Prow CI** | Current | Automated testing infrastructure | KubeVirt's official CI system, runs all presubmit checks |
| **Bazel** | Current | Build system | Required for building KubeVirt, handles all compilation |
| **Ginkgo v2** | Current | BDD test framework | All tests use Ginkgo with label-based filtering |
| **Gomega** | Current | Assertion library | Paired with Ginkgo for all test assertions |
| **gomock** | Current | Mock generation | Standard for unit test mocking (e.g., kubecli mocks) |
| **kubevirtci** | Current | Local test clusters | Provisions ephemeral K8s clusters for testing |

### Testing Infrastructure

| Component | Version | Purpose | When to Use |
|-----------|---------|---------|-------------|
| **Unit tests** | `make test` | Fast, no cluster required | Always - required for all PRs |
| **Functional tests** | `make functest` | E2E tests on local cluster | Bug fixes and features touching runtime behavior |
| **Prow test lanes** | k8s-1.29, k8s-1.30 | Multi-version K8s validation | Automatic on approved PRs |
| **Code generation** | `make generate` | API/mock regeneration | After API or interface changes |

### Development Commands

**Local development:**
```bash
# Build project
make

# Run unit tests
make test

# Spin up local cluster
make cluster-up

# Deploy changes
make cluster-sync

# Run functional tests
make functest

# Focused test run (remove F prefix before submitting!)
FUNC_TEST_ARGS='--focus-file=vmi_test' make functest

# Regenerate code after API changes
make generate
```

**Git workflow:**
```bash
# Sign commits (REQUIRED)
git commit -s -m "fix(hotplug): prevent race in cleanupAttachmentPods"

# Fix forgotten DCO on last commit
git commit --amend --no-edit --signoff

# Bulk sign all commits from main
git rebase --exec 'git commit --amend --no-edit -n -s' -i main
```

### Alternatives Considered

| Standard Approach | Alternative | Tradeoff |
|-------------------|-------------|----------|
| Prow CI | GitHub Actions | Prow is KubeVirt's infrastructure - no choice |
| Bazel build | Standard go build | Bazel required for proper builds, handles complex deps |
| Ginkgo v2 labels | Text-based test labels | Labels enable better filtering, cleaner descriptions |
| Local functest | Skip local testing | CI will catch issues but slower feedback loop |

## Architecture Patterns

### Recommended PR Structure

For bug fix PRs like the hotplug race condition:

```
kubevirt/                              # Fork root
├── pkg/virt-controller/
│   └── watch/vmi/
│       ├── volume-hotplug.go          # Fix implementation
│       └── vmi_test.go                # Unit tests (same file pattern)
└── .planning/                         # NOT submitted upstream
    └── phases/09-implement-fix/       # Local planning docs
        ├── 09-01-CODEPATH.md          # Use for PR description content
        ├── 09-03-VALIDATION.md        # Reference in PR description
        └── 09-VERIFICATION.md
```

**Key principles:**
- Keep fix and tests in same subsystem (`pkg/virt-controller/watch/vmi/`)
- Add tests to existing test files (`vmi_test.go`) not new files
- Use planning docs to inform PR description, don't submit them
- Single logical change per PR (the race fix, not multiple hotplug improvements)

### Pattern 1: Bug Fix PR with Unit Tests

**What:** Small, focused bug fix with comprehensive test coverage

**When to use:** Race conditions, logic errors, edge case handling - anything reproducible in unit tests

**Structure:**
```go
// Fix implementation in volume-hotplug.go
// allHotplugVolumesReady checks if all hotplug volumes have reached VolumeReady phase
func allHotplugVolumesReady(vmi *v1.VirtualMachineInstance) bool {
    for _, vol := range vmi.Spec.Volumes {
        if vol.DataVolume == nil && vol.PersistentVolumeClaim == nil {
            continue // Skip non-hotplug volumes
        }
        // Check VolumeStatus for Ready phase
        found := false
        for _, status := range vmi.Status.VolumeStatus {
            if status.Name == vol.Name && status.Phase == v1.VolumeReady {
                found = true
                break
            }
        }
        if !found {
            return false
        }
    }
    return true
}

// Modified cleanup logic
func (c *VMIController) cleanupAttachmentPods(vmi *v1.VirtualMachineInstance, ...) error {
    if len(oldPods) > 0 {
        // NEW: Early return if volumes not ready (prevents race)
        if !allHotplugVolumesReady(vmi) {
            return nil
        }
        // Existing cleanup logic...
    }
}
```

```go
// Unit tests in vmi_test.go
var _ = Describe("Volume hotplug attachment pod cleanup", func() {
    It("should NOT delete old pod when new pod volumes are not ready (bug reproduction)", func() {
        // Setup: VMI with hotplug volume in non-Ready state
        vmi := createVMIWithHotplugVolume("test-vol", v1.VolumeBound) // Not VolumeReady

        // Execute cleanup
        syncErr := controller.cleanupAttachmentPods(vmi, ...)

        // Assert: Old pod should NOT be deleted
        Expect(syncErr).ToNot(HaveOccurred())
        Expect(deletedPods).To(BeEmpty()) // Key assertion
    })

    It("should delete old pod when all volumes are ready", func() {
        // Setup: VMI with hotplug volume in Ready state
        vmi := createVMIWithHotplugVolume("test-vol", v1.VolumeReady)

        // Execute cleanup
        syncErr := controller.cleanupAttachmentPods(vmi, ...)

        // Assert: Old pod SHOULD be deleted
        Expect(syncErr).ToNot(HaveOccurred())
        Expect(deletedPods).To(HaveLen(1))
    })
})
```

**Source:** Pattern derived from [virt-controller test structure](https://github.com/kubevirt/kubevirt/blob/main/pkg/virt-controller/watch/vmi_test.go) and [unit test howto](https://kubevirt.io/2018/Unit-Test-Howto.html)

### Pattern 2: PR Description Structure

**What:** Standard KubeVirt PR template format

**When to use:** Every PR submission

**Template:**
```markdown
**What this PR does / why we need it:**

Fixes a race condition in volume hotplug where concurrent hotplugging of
multiple volumes causes the virt-controller to delete old attachment pods
before new volumes reach the Ready phase, resulting in VM pause and I/O errors.

**Before:** cleanupAttachmentPods() deleted old pods immediately when new
pods existed, regardless of volume readiness state.

**After:** cleanupAttachmentPods() waits until all hotplug volumes reach
VolumeReady phase before deleting old attachment pods.

**Which issue(s) this PR fixes:**

Fixes kubevirt/kubevirt#6564
Fixes kubevirt/kubevirt#9708
Related to kubevirt/kubevirt#16520

**Special notes for your reviewer:**

Validated on production-like cluster with nested K3s worker VM. Multi-volume
concurrent hotplug now works without VM pause or I/O errors. See manual
validation results in PR description.

**Release note:**
```release-note
Fix race condition in volume hotplug that caused VM pause and I/O errors
when hotplugging multiple volumes concurrently
```
```

**Source:** Derived from [KubeVirt PR #12800](https://github.com/kubevirt/kubevirt/pull/12800) and [CONTRIBUTING.md checklist](https://github.com/kubevirt/kubevirt/blob/main/CONTRIBUTING.md)

### Pattern 3: Commit Message Format

**What:** Conventional commit style with DCO sign-off

**When to use:** Every commit

**Format:**
```
<type>(<scope>): <subject>

<body - optional but recommended for non-trivial changes>

Fixes #<issue>
Signed-off-by: Your Name <email@example.com>
```

**Example:**
```
fix(hotplug): wait for new pod volumes ready before deleting old pod

Adds allHotplugVolumesReady() helper that checks if all hotplug volumes
have reached VolumeReady phase. Modified cleanupAttachmentPods() to early
return if volumes not ready, preventing premature deletion of old
attachment pods during concurrent volume hotplug operations.

This prevents VM pause and I/O errors when hotplugging multiple volumes
simultaneously, as reported in #6564 and #9708.

Fixes #6564
Fixes #9708

Signed-off-by: Your Name <email@example.com>
```

**Source:** [DCO sign-off documentation](https://github.com/kubevirt/kubevirt/labels/dco-signoff:%20yes) and commit message best practices

### Anti-Patterns to Avoid

- **Large monolithic PRs:** Keep PRs to 200-400 LOC. Multi-volume fix + refactoring = 2 PRs
- **Missing DCO sign-off:** All commits MUST have `Signed-off-by` line or CI blocks merge
- **Focused test markers in final PR:** Remove `FDescribe`, `FIt` prefixes before submission
- **Skipping local tests:** Always run `make test` and `make functest` locally before pushing
- **Missing release notes:** Bug fixes need release notes explaining user-visible impact
- **Vague PR descriptions:** "Fix hotplug bug" vs "Fix race causing VM pause in concurrent hotplug"
- **Not rebasing:** PRs with merge conflicts get `needs-rebase` label and won't merge

## Don't Hand-Roll

Problems that look simple but have existing solutions or established patterns:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| **Local test cluster** | Custom K8s setup scripts | `make cluster-up` with kubevirtci | Handles multi-version K8s, networking, nested virt |
| **Mock K8s clients** | Manual interface mocking | gomock with `kubecli.NewMockKubevirtClient()` | Generated mocks match actual interfaces |
| **Test fixtures** | Ad-hoc VMI creation | Existing test helpers in `tests/` package | Consistent test data across test suites |
| **DCO sign-off** | Manual signature editing | `git commit -s` or `--signoff` flag | Prevents formatting errors, automation-friendly |
| **Code generation** | Manual API updates | `make generate` after changes | Regenerates mocks, clients, deep copies correctly |
| **CI debugging** | Guessing what tests run | Check Prow dashboard at prow.ci.kubevirt.io | Shows exact test lanes and failure logs |

**Key insight:** KubeVirt has well-established tooling and patterns. First-time contributors should follow existing patterns in `vmi_test.go` and other test files rather than inventing new test structures.

## Common Pitfalls

### Pitfall 1: Forgetting DCO Sign-Off

**What goes wrong:** PR blocked with "dco-signoff: no" label, can't merge until all commits signed

**Why it happens:** Contributors used to `git commit -m` without `-s` flag

**How to avoid:**
- Always use `git commit -s` or `git commit --signoff`
- Configure Git alias: `git config alias.cs 'commit -s'`
- Pre-commit hook to check for sign-off

**Warning signs:**
- PR has "dco-signoff: no" label
- CI check fails with "missing DCO sign-off"

**Fix:**
```bash
# Last commit only
git commit --amend --no-edit --signoff
git push -f origin branch-name

# Multiple commits from main
git rebase --exec 'git commit --amend --no-edit -n -s' -i main
git push -f origin branch-name
```

### Pitfall 2: New Contributor CI Approval Wait

**What goes wrong:** Push commits, no CI tests run, PR sits idle

**Why it happens:** First-time contributors need manual approval with `/ok-to-test` from org member

**How to avoid:**
- Clearly state in PR description this is first contribution
- Ping in #kubevirt-dev Slack if no response after 24-48 hours
- Ensure PR is NOT a draft (drafts skip CI intentionally)

**Warning signs:**
- PR has `needs-ok-to-test` label
- No Prow test jobs triggered after push
- "Awaiting CI" status for >24 hours

**Fix:** Politely comment requesting review, mention this is first contribution and tests are ready

### Pitfall 3: Bazel Server Stale State

**What goes wrong:** `make` or `make test` fails with mysterious errors after branch switch

**Why it happens:** Bazel server caches state across branch changes

**How to avoid:**
- Stop Bazel server after major branch switches: `docker stop kubevirt-bazel-server`
- Use `make clean` before rebuild after checkout

**Warning signs:**
- Build errors that don't match code changes
- "Target not found" errors for files that exist
- Tests fail locally but pass in CI

**Fix:**
```bash
docker stop kubevirt-bazel-server
make clean
make test
```

### Pitfall 4: Missing Rebase on Main

**What goes wrong:** PR gets `needs-rebase` label, maintainers can't merge

**Why it happens:** Main branch advanced while PR was open, now has conflicts

**How to avoid:**
- Rebase on main before opening PR
- Monitor PR for `needs-rebase` label
- Rebase promptly when requested

**Warning signs:**
- `needs-rebase` label appears
- GitHub shows "This branch has conflicts"
- Maintainer comments "Please rebase"

**Fix:**
```bash
git fetch upstream
git rebase upstream/main
# Resolve conflicts if any
git push -f origin branch-name
```

### Pitfall 5: Leaving Focused Test Markers

**What goes wrong:** PR submitted with `FDescribe`, `FIt` in test code, CI runs only subset of tests

**Why it happens:** Used `F` prefixes for local focused testing, forgot to remove

**How to avoid:**
- Search for `FDescribe\|FIt\|FContext` before `git add`
- Pre-commit hook to block `F`-prefixed test functions
- Code review checklist item

**Warning signs:**
- Fewer tests run in CI than expected
- Maintainer comments about focused tests
- Local test count differs from CI

**Fix:**
```bash
# Find focused tests
grep -r "FDescribe\|FIt\|FContext" pkg/

# Remove F prefix
# FIt("test") -> It("test")
```

### Pitfall 6: Vague or Missing Release Notes

**What goes wrong:** Maintainer requests release note update, delays merge

**Why it happens:** Contributor doesn't understand user-facing impact or thinks bug fix doesn't need notes

**How to avoid:**
- Bug fixes almost always need release notes (users upgrading want to know what's fixed)
- Use clear, non-technical language: "Fix VM pause during volume hotplug" not "Fix race in cleanupAttachmentPods"
- Include action required if users need to do something

**Warning signs:**
- `/release-note-none` when fix impacts users
- Release note uses internal terminology
- Maintainer asks "Can you clarify the release note?"

**Fix:**
```markdown
```release-note
Fix race condition that caused VMs to pause and experience I/O errors
when hotplugging multiple volumes concurrently
```
```

## Code Examples

Verified patterns from official sources:

### Creating a Unit Test for Bug Reproduction

```go
// Source: pkg/virt-controller/watch/vmi/vmi_test.go patterns
var _ = Describe("Volume hotplug", func() {
    var ctrl *gomock.Controller
    var vmiInterface *kubecli.MockVirtualMachineInstanceInterface
    var controller *VMIController

    BeforeEach(func() {
        ctrl = gomock.NewController(GinkgoT())
        vmiInterface = kubecli.NewMockVirtualMachineInstanceInterface(ctrl)
        // Setup controller with mocks...
    })

    AfterEach(func() {
        ctrl.Finish()
    })

    Context("attachment pod cleanup", func() {
        It("should NOT delete old pod when volumes are not ready", func() {
            // Arrange: Create VMI with volume in Bound state (not Ready)
            vmi := api.NewMinimalVMI("test-vmi")
            vmi.Spec.Volumes = []v1.Volume{{
                Name: "hotplug-vol",
                VolumeSource: v1.VolumeSource{
                    PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
                        ClaimName: "test-pvc",
                    },
                },
            }}
            vmi.Status.VolumeStatus = []v1.VolumeStatus{{
                Name:  "hotplug-vol",
                Phase: v1.VolumeBound, // Not VolumeReady!
            }}

            // Act: Trigger cleanup
            syncErr := controller.cleanupAttachmentPods(vmi, pod)

            // Assert: No deletion should occur
            Expect(syncErr).ToNot(HaveOccurred())
            // Verify no delete calls on client mocks
        })
    })
})
```

**Source:** [vmi_test.go test patterns](https://github.com/kubevirt/kubevirt/blob/main/pkg/virt-controller/watch/vmi_test.go) and [Unit Test Howto](https://kubevirt.io/2018/Unit-Test-Howto.html)

### Helper Functions for Test Setup

```go
// createVMIWithHotplugVolume creates a VMI with a single hotplug volume in specified phase
func createVMIWithHotplugVolume(volumeName string, phase v1.VolumePhase) *v1.VirtualMachineInstance {
    vmi := api.NewMinimalVMI("test-vmi")
    vmi.Spec.Volumes = []v1.Volume{{
        Name: volumeName,
        VolumeSource: v1.VolumeSource{
            PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
                ClaimName: volumeName + "-pvc",
            },
        },
    }}
    vmi.Status.VolumeStatus = []v1.VolumeStatus{{
        Name:   volumeName,
        Phase:  phase,
        Target: volumeName,
    }}
    return vmi
}

// createMultiVolumeVMI creates VMI with multiple hotplug volumes
func createMultiVolumeVMI(volumes map[string]v1.VolumePhase) *v1.VirtualMachineInstance {
    vmi := api.NewMinimalVMI("test-vmi")
    for name, phase := range volumes {
        vmi.Spec.Volumes = append(vmi.Spec.Volumes, v1.Volume{
            Name: name,
            VolumeSource: v1.VolumeSource{
                PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
                    ClaimName: name + "-pvc",
                },
            },
        })
        vmi.Status.VolumeStatus = append(vmi.Status.VolumeStatus, v1.VolumeStatus{
            Name:  name,
            Phase: phase,
        })
    }
    return vmi
}
```

**Source:** Common test helper pattern in KubeVirt tests

### Running Local Tests Before PR

```bash
# Full pre-submission workflow
cd /path/to/kubevirt-fork

# 1. Ensure code is formatted
make fmt

# 2. Run unit tests
make test

# 3. Regenerate code if you changed APIs
make generate

# 4. Start local cluster (first time or after cluster-down)
make cluster-up

# 5. Deploy your changes
make cluster-sync

# 6. Run functional tests (focused on your area)
FUNC_TEST_ARGS='--focus-file=vmi_test' make functest

# 7. Optional: Run specific test
FUNC_TEST_ARGS='--ginkgo.focus="should NOT delete old pod"' make functest

# 8. Clean up cluster when done
make cluster-down
```

**Source:** [Getting Started Guide](https://github.com/kubevirt/kubevirt/blob/main/docs/getting-started.md)

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Text-based test labels in descriptions | Ginkgo v2 label decorators | ~2023 | Better filtering, cleaner test descriptions |
| Manual DCO sign-off editing | `git commit -s` flag | Always required | Automated, fewer format errors |
| Single K8s version testing | Multi-version test lanes (1.29, 1.30) | Ongoing | Better compatibility coverage |
| `/lgtm` and `/approve` separately | Two-phase review (reviewer + approver) | OWNERS-based | Clearer review roles |
| Draft PRs run CI | Draft PRs skip CI | Recent | Conserves CI resources |

**Deprecated/outdated:**
- **Bazel osxkeychain on Mac:** Disable in `~/.docker/config.json` - Bazel doesn't support it
- **`-uall` flag in git status:** Never use, causes memory issues on large repos
- **Ginkgo v1 text labels:** Use v2 label decorators instead
- **Single functional test run:** Now organized into SIG-based lanes (sig-compute, sig-storage, sig-network)

## Open Questions

Things that couldn't be fully resolved:

1. **E2E Test Requirement for Bug Fixes**
   - What we know: Unit tests required, functional tests "almost always" required
   - What's unclear: Is e2e test strictly required for virt-controller logic bug, or is comprehensive unit test sufficient?
   - Recommendation: Include strong unit tests covering bug reproduction + fix validation. Mention manual validation in PR description. If maintainers request e2e, add it in revision.

2. **VEP Requirement Threshold**
   - What we know: Bug fixes and simple changes don't need VEP. Complex features, new APIs, architectural changes need VEP.
   - What's unclear: Exact line where "bug fix with new helper function" becomes "enhancement requiring VEP"
   - Recommendation: Hotplug race fix is clearly a bug fix (addresses reported issues #6564, #9708). No VEP needed.

3. **Maintainer Response Time Expectations**
   - What we know: Average PR lifespan is 3 days, project has fast merge rate
   - What's unclear: When to ping maintainers if no initial response
   - Recommendation: Wait 24-48 hours for initial review. If no response, politely ping in PR or #kubevirt-dev Slack. Include timezone note if relevant.

4. **Test Coverage Requirements**
   - What we know: "Untested features do not exist" is the principle. 80% coverage is often sufficient, 100% is expensive.
   - What's unclear: Specific coverage percentage required for merge
   - Recommendation: Ensure the bug is reproducible in tests (fail before fix, pass after). Cover edge cases (single volume, multi-volume, removal). Don't aim for arbitrary coverage percentage.

## Sources

### Primary (HIGH confidence)

- [KubeVirt CONTRIBUTING.md](https://github.com/kubevirt/kubevirt/blob/main/CONTRIBUTING.md) - Official contribution guidelines
- [KubeVirt Getting Started](https://github.com/kubevirt/kubevirt/blob/main/docs/getting-started.md) - Development environment setup and testing
- [KubeVirt Contributing Page](https://kubevirt.io/user-guide/contributing/) - Community workflow and governance
- [vmi_test.go](https://github.com/kubevirt/kubevirt/blob/main/pkg/virt-controller/watch/vmi_test.go) - Test patterns for virt-controller
- [KubeVirt PR #12800](https://github.com/kubevirt/kubevirt/pull/12800) - Example successful hotplug-related PR
- [KubeVirt enhancements README](https://github.com/kubevirt/enhancements/blob/main/README.md) - VEP process documentation

### Secondary (MEDIUM confidence)

- [Kubernetes OWNERS Files](https://www.kubernetes.dev/docs/guide/owners/) - OWNERS file usage (applies to KubeVirt)
- [Kubernetes Commit Message Guide](https://www.kubernetes.dev/docs/guide/release-notes/) - Release note format (KubeVirt follows similar pattern)
- [KubeVirt Unit Test Howto](https://kubevirt.io/2018/Unit-Test-Howto.html) - Testing philosophy (older but still relevant)
- [KubeVirt Prow CI](https://prow.ci.kubevirt.io) - CI infrastructure and test lanes
- [kubevirtci repository](https://github.com/kubevirt/kubevirtci) - Local cluster provisioning

### Tertiary (LOW confidence - community observations)

- [kubevirt-dev mailing list](https://groups.google.com/g/kubevirt-dev) - VEP discussions and community questions
- [Kubernetes faster reviews](https://github.com/kubernetes/kubernetes/blob/release-1.5/docs/devel/faster_reviews.md) - General PR best practices
- Multiple KubeVirt PRs examined for patterns - real-world examples of successful contributions

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Official docs, direct repository inspection, established tooling
- Architecture: HIGH - Examined actual successful PRs, official test patterns, documented workflows
- Pitfalls: HIGH - DCO and rebase issues well-documented, CI approval process in CONTRIBUTING.md
- E2E test requirements: MEDIUM - Principle clear ("almost always"), exact requirement for this fix type unclear
- VEP requirements: HIGH - Bug fix exemption documented in enhancements README

**Research date:** 2026-01-31
**Valid until:** 2026-04-30 (90 days - KubeVirt project is stable, major process unlikely to change rapidly)

**Key constraints from Phase 9:**
- Fix already implemented in commit cc1b700 on hotplug-fix-v1 branch
- Unit tests already added in commit 6546421 (5 test cases)
- Fork location: https://github.com/whiskey-works/kubevirt
- Related issues: #6564, #9708, #16520
- Fix validated on metal cluster with nested K3s worker

**Phase 10 planning should address:**
1. PR preparation: Clean commit history, DCO sign-off, proper commit messages
2. PR description: Leverage 09-01-CODEPATH.md and 09-03-VALIDATION.md content
3. Release notes: User-facing description of fix
4. Initial submission: How to get `/ok-to-test` approval as first-time contributor
5. Review feedback handling: Rebase, regenerate code, address maintainer comments
6. Merge readiness: All CI lanes pass, approver LGTM
