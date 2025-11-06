# Security Fixes - Action Items

This document tracks the security issues identified in the comprehensive security audit and their remediation status.

## Critical Priority (Fix Immediately)

### 1. SSH Host Key Verification
- [ ] Store RDS host key in Kubernetes Secret
- [ ] Implement host key verification in SSH client
- [ ] Make verification mandatory (remove InsecureIgnoreHostKey)
- [ ] Add host key mismatch detection and alerting
- [ ] Update documentation with host key setup instructions
- [ ] Update deployment manifests to include host key secret

**Files to modify:**
- `pkg/rds/ssh_client.go`
- `pkg/rds/client.go`
- `deploy/kubernetes/controller.yaml`
- `docs/kubernetes-setup.md`

**Estimated effort:** 4-6 hours

---

### 2. File Path Command Injection Prevention
- [ ] Add `validateFilePath()` function in `pkg/utils/`
- [ ] Check for shell metacharacters: `;`, `|`, `&`, `$`, `` ` ``, `(`, `)`, `<`, `>`, `\n`, `\r`
- [ ] Implement whitelist of allowed base paths
- [ ] Add path traversal protection using `filepath.Clean()`
- [ ] Call validation in `validateCreateVolumeOptions()`
- [ ] Add unit tests for path validation
- [ ] Add integration tests with malicious paths

**Files to modify:**
- `pkg/utils/validation.go` (new file)
- `pkg/rds/commands.go`
- `pkg/utils/validation_test.go` (new file)

**Estimated effort:** 4-6 hours

---

## High Priority (Fix within 1 week)

### 3. NQN Format Validation
- [x] Add NQN regex validation: `^nqn\.[0-9]{4}-[0-9]{2}\.[a-z0-9.-]+:[a-z0-9._-]+$`
- [x] Validate NQN in `VolumeIDToNQN()` function
- [x] Add NQN validation before nvme connect operations
- [x] Add unit tests for NQN validation
- [x] Reject NQNs with shell metacharacters

**Files to modify:**
- `pkg/utils/volumeid.go`
- `pkg/nvme/nvme.go`
- `pkg/utils/volumeid_test.go`

**Estimated effort:** 2-3 hours

---

### 4. Reduce Container Privileges
- [ ] Remove `privileged: true` if possible
- [ ] Test with only `SYS_ADMIN` capability
- [ ] Add `CAP_NET_ADMIN` for NVMe/TCP
- [ ] Drop all other capabilities
- [ ] Set `allowPrivilegeEscalation: false`
- [ ] Add `readOnlyRootFilesystem: true`
- [ ] Test with `runAsNonRoot: true`
- [ ] Add seccomp profile
- [ ] Add AppArmor/SELinux profile
- [ ] Update node DaemonSet manifest

**Files to modify:**
- `deploy/kubernetes/node.yaml`
- Test thoroughly on actual hardware

**Estimated effort:** 8-12 hours (requires extensive testing)

---

### 5. Mount Options Validation
- [ ] Create mount options whitelist
- [ ] Implement `validateMountOptions()` function
- [ ] Reject dangerous options: `suid`, `dev`, `exec`
- [ ] Enforce `nosuid,nodev,noexec` by default for bind mounts
- [ ] Add configuration for allowed mount options
- [ ] Add unit tests
- [ ] Log all mount operations with options

**Files to modify:**
- `pkg/mount/mount.go`
- `pkg/driver/node.go`
- `pkg/mount/mount_test.go`

**Estimated effort:** 3-4 hours

---

### 6. Volume Context Parameter Validation
- [ ] Add IP address validation for nvmeAddress
- [ ] Add port range validation (1-65535, not privileged)
- [ ] Verify nvmeAddress matches expected RDS address
- [ ] Add NQN format validation for context NQN
- [ ] Implement allowlist for NVMe target addresses
- [ ] Add validation function in NodeStageVolume
- [ ] Add unit tests

**Files to modify:**
- `pkg/driver/node.go`
- `pkg/utils/validation.go`

**Estimated effort:** 3-4 hours

---

## Medium Priority (Fix within 1 month)

### 7. SSH Connection Rate Limiting
- [ ] Implement connection pool with max size
- [ ] Add rate limiter using `golang.org/x/time/rate`
- [ ] Implement circuit breaker pattern
- [ ] Add configurable timeout for all SSH operations
- [ ] Consider connection reuse/multiplexing
- [ ] Add metrics for connection pool usage

**Files to modify:**
- `pkg/rds/ssh_client.go`
- `pkg/rds/pool.go` (new file)

**Estimated effort:** 6-8 hours

---

### 8. Error Message Sanitization
- [ ] Create error classification (internal vs. user-facing)
- [ ] Implement error sanitization function
- [ ] Remove internal paths from user errors
- [ ] Remove IP addresses from user errors
- [ ] Keep detailed errors in logs only
- [ ] Update all error returns to use sanitized errors

**Files to modify:**
- Multiple files throughout codebase
- `pkg/utils/errors.go` (new file)

**Estimated effort:** 8-10 hours

---

### 9. NVMe Operation Timeouts
- [ ] Add context with timeout to nvme-cli commands
- [ ] Make timeout configurable
- [ ] Add healthcheck for stuck operations
- [ ] Implement automatic cleanup of hung operations
- [ ] Add metrics for operation duration

**Files to modify:**
- `pkg/nvme/nvme.go`

**Estimated effort:** 3-4 hours

---

### 10. Regex Optimization (ReDoS Prevention)
- [ ] Review all regex patterns for complexity
- [ ] Simplify or replace complex patterns with string parsing
- [ ] Add input length limits before regex matching
- [ ] Add timeout wrapper for regex operations
- [ ] Benchmark regex performance

**Files to modify:**
- `pkg/rds/commands.go`

**Estimated effort:** 4-6 hours

---

### 11. Enhanced Security Logging
- [ ] Implement structured logging for security events
- [ ] Log all authentication attempts
- [ ] Log all volume operations with identity
- [ ] Add security event classification
- [ ] Implement metrics for security events
- [ ] Add log aggregation documentation

**Files to modify:**
- Multiple files throughout codebase
- Add centralized security logger

**Estimated effort:** 6-8 hours

---

### 12. Container Image Signing
- [ ] Set up cosign in CI/CD pipeline
- [ ] Sign images on build
- [ ] Add signature verification to deployment
- [ ] Use image digests instead of tags
- [ ] Document verification process
- [ ] Add vulnerability scanning to CI/CD

**Files to modify:**
- `.github/workflows/` or CI configuration
- `deploy/kubernetes/controller.yaml`
- `deploy/kubernetes/node.yaml`

**Estimated effort:** 4-6 hours

---

### 13. Secrets Encryption Documentation
- [ ] Document encryption at rest requirements
- [ ] Provide encryption configuration examples
- [ ] Document secret rotation procedures
- [ ] Add Vault integration example
- [ ] Document backup security

**Files to modify:**
- `docs/kubernetes-setup.md`
- `docs/security.md` (new file)

**Estimated effort:** 2-3 hours

---

## Low Priority (Fix as time permits)

### 14. Command Output Size Limits
- [ ] Add maximum output size limit (1MB)
- [ ] Implement streaming for large outputs
- [ ] Add output size monitoring
- [ ] Update container resource limits

**Files to modify:**
- `pkg/rds/ssh_client.go`

**Estimated effort:** 2-3 hours

---

### 15. Directory Permission Hardening
- [ ] Use `0700` instead of `0750` for staging directories
- [ ] Review all permission assignments
- [ ] Document permission requirements

**Files to modify:**
- `pkg/mount/mount.go`

**Estimated effort:** 1 hour

---

### 16. Log Sanitization
- [ ] Implement log redaction for sensitive data
- [ ] Add security classification to logs
- [ ] Avoid logging full command outputs at INFO level
- [ ] Document log access controls

**Files to modify:**
- Throughout codebase

**Estimated effort:** 4-6 hours

---

## Implementation Plan

### Phase 1 (Week 1): Critical Fixes
- SSH host key verification
- File path validation
- Mount options validation

### Phase 2 (Week 2-3): High Priority Fixes
- NQN validation
- Container privilege reduction
- Volume context validation

### Phase 3 (Month 2): Medium Priority Fixes
- Rate limiting
- Error sanitization
- Timeouts and logging
- Container signing

### Phase 4 (Month 3): Polish and Documentation
- Remaining medium priority items
- Low priority fixes
- Security documentation
- Penetration testing

---

## Testing Strategy

For each fix:
1. Write unit tests before implementation
2. Add integration tests where applicable
3. Test with real RDS hardware
4. Run CSI sanity tests
5. Document test cases

---

## Success Criteria

- [ ] All critical issues resolved
- [ ] All high priority issues resolved
- [ ] 90%+ of medium priority issues resolved
- [ ] Security testing passed
- [ ] Documentation updated
- [ ] SECURITY-AUDIT-REPORT.md updated with remediation status

---

## Notes

- Each fix should be in a separate commit/PR for easier review
- Add "Security:" prefix to commit messages for security fixes
- Update ROADMAP.md as fixes are completed
- Consider creating GitHub Security Advisories for critical issues once fixed
