# RDS CSI Driver - Helper Scripts

This directory contains helper scripts for development, deployment, and testing.

## Scripts

### deploy-dev.sh

Deploys the RDS CSI driver with the `:dev` container image tag.

**Usage:**
```bash
# Deploy and continue
./scripts/deploy-dev.sh

# Deploy and wait for rollout to complete
./scripts/deploy-dev.sh --wait
```

**What it does:**
1. Updates the controller deployment image to `ghcr.io/3whiskeywhiskey/rds-csi-driver:dev`
2. Updates the node daemonset image to `ghcr.io/3whiskeywhiskey/rds-csi-driver:dev`
3. Shows current pod status
4. Optionally waits for rollout completion

**When to use:**
- After merging a security fix to the dev branch
- After CI/CD builds and pushes a new `:dev` image
- Testing development builds in your cluster

---

### test-security-fix.sh

Comprehensive testing suite for security fixes.

**Usage:**
```bash
# Full test suite for fix #1
./scripts/test-security-fix.sh 1

# Skip PVC lifecycle test
./scripts/test-security-fix.sh 2 --skip-lifecycle

# Skip integration tests
./scripts/test-security-fix.sh 3 --skip-integration
```

**What it tests:**
1. **Deployment Health** - Verifies controller and node pods are ready
2. **PVC Lifecycle** - Creates test PVC, writes data, verifies, cleans up
3. **Integration Tests** - Runs `make test-integration`
4. **Log Inspection** - Checks controller and node logs for errors
5. **Security Validation** - Fix-specific security checks

**Test PVC naming:**
- Test PVCs are named `test-security-fix-<number>`
- They are automatically cleaned up after testing
- Your existing PVCs (pvc-0d4aa0d9-*, etc.) are never touched

**Security Fix Numbers:**
- 1 - SSH host key verification
- 2 - File path validation
- 3 - NQN validation
- 4 - Container privileges
- 5 - Mount options validation
- 6 - Volume context validation
- 7 - Rate limiting
- 8 - Error sanitization
- 9 - NVMe timeouts
- 10 - ReDoS prevention
- 11 - RBAC + Image signing

---

## Workflow: Merging Security Fixes

Follow this workflow for each security fix:

### Step 1: Merge the fix to dev
```bash
# Merge security fix #1
git checkout dev
git merge github/claude/security-fix-1-ssh-host-key-011CUsQLcQ6A5UvW4camcWt1
git push github dev
```

### Step 2: Wait for CI/CD
GitHub Actions will automatically:
- Run `make verify` (format, vet, lint, test)
- Run `make test-integration`
- Build multi-arch image (amd64/arm64)
- Push to `ghcr.io/3whiskeywhiskey/rds-csi-driver:dev`

Monitor the build: https://github.com/3whiskeywhiskey/rds-csi-driver/actions

### Step 3: Deploy to cluster
```bash
# Deploy the new :dev image
./scripts/deploy-dev.sh --wait
```

### Step 4: Test thoroughly
```bash
# Run comprehensive test suite
./scripts/test-security-fix.sh 1
```

### Step 5: Verify and continue
If tests pass:
- Review logs for warnings
- Check existing PVCs are healthy
- Proceed to next security fix (repeat from Step 1)

If tests fail:
- Investigate the issue
- Fix the problem
- Rebuild and redeploy
- Test again

---

## Environment Requirements

**kubectl:**
- Must be configured to access your cluster
- Must have permissions to update deployments/daemonsets

**Container Registry:**
- GitHub Container Registry (ghcr.io)
- Images pushed by GitHub Actions with `:dev` tag

**Kubernetes Resources:**
- Namespace: `kube-system` (for driver)
- StorageClass: `rds-csi` (for test PVCs)
- Test namespace: `default` (for test pods)

---

## Troubleshooting

### Deploy fails: "cannot connect to cluster"
```bash
# Check kubectl configuration
kubectl cluster-info

# Verify you can access the cluster
kubectl get nodes
```

### Test fails: PVC stuck in Pending
```bash
# Check StorageClass exists
kubectl get storageclass rds-csi

# Check controller logs
kubectl logs -n kube-system deployment/rds-csi-controller

# Check events
kubectl describe pvc test-security-fix-<number>
```

### Test cleanup fails
```bash
# Manually clean up test resources
kubectl delete pod test-pod-security-fix-<number> --force --grace-period=0
kubectl delete pvc test-security-fix-<number> --force --grace-period=0
```

### CI/CD build fails
- Check GitHub Actions logs
- Verify `make verify` passes locally
- Ensure all tests pass: `make test && make test-integration`

---

## Development Workflow

For local development without CI/CD:

```bash
# Build locally
make build-local

# Build and push :dev image manually
make docker-push IMAGE_TAG=dev

# Deploy to cluster
./scripts/deploy-dev.sh --wait

# Test
./scripts/test-security-fix.sh <fix-number>
```

---

## Safety Notes

- **Existing PVCs are safe**: Test scripts use separate PVC names
- **Rolling updates**: Node daemonset updates pods one at a time
- **Cleanup**: Test resources are automatically cleaned up
- **Monitoring**: Scripts show pod status and provide monitoring commands

For questions or issues, see the main project documentation.
