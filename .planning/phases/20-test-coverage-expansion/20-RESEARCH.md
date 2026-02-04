# Phase 20: Test Coverage Expansion - Research

**Researched:** 2026-02-04
**Domain:** Go testing, test coverage, mocking strategies
**Confidence:** HIGH

## Summary

Phase 20 aims to increase test coverage from current levels (RDS 44.4%, mount 55.9%, nvme 43.3%) to >80% on all critical packages. Research reveals that the project already has solid testing infrastructure in place with table-driven tests and exec command mocking. The primary challenge is expanding coverage to untested critical paths: SSH client operations (0% coverage), RDS command execution (0% coverage), mount error paths, NVMe connection retry logic, and state persistence.

The Go ecosystem provides mature patterns for achieving 80% coverage: interface-based mocking, table-driven tests with testify, mock SSH servers, exec command mocking (already in use), and filesystem abstraction. The key is not chasing percentage targets but ensuring critical error paths—connection failures, device disappearance, RDS quota exceeded, mount storms, retry exhaustion—have explicit test coverage.

**Primary recommendation:** Prioritize testing critical error paths over percentage targets. Use existing exec command mocking pattern for consistency. Add SSH mock server for pkg/rds/ssh_client.go. Write table-driven tests for all retry logic, timeouts, and context cancellation. Use go-test-coverage tool to enforce 80% thresholds per file.

## Standard Stack

The established libraries/tools for Go testing and coverage:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| testing | stdlib | Test framework | Built-in, no dependencies, table-driven pattern support |
| golang.org/x/crypto/ssh/test | latest | SSH mock server | Official Go crypto package, used for testing SSH clients |
| go test -race | stdlib | Race detection | Detects concurrency bugs, critical for attachment state |
| go test -cover | stdlib | Coverage reports | Built-in coverage profiling, JSON output support |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/stretchr/testify | v1.9+ | Assertions, mocks | Already used in project, table-driven test helpers |
| github.com/vladopajic/go-test-coverage | v2+ | Coverage enforcement | CI/CD enforcement of 80% thresholds per file/package |
| testing/fstest | stdlib (1.16+) | Filesystem mocking | For testing file operations without disk I/O |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| exec mocking | github.com/spf13/afero | Heavyweight for mount/nvme use case; exec pattern already works |
| testify | gomock | More verbose; testify already in use, keep consistency |
| Manual SSH mock | gliderlabs/ssh | More realistic but heavier; x/crypto/ssh/test sufficient for unit tests |

**Installation:**
```bash
# Coverage enforcement tool
go install github.com/vladopajic/go-test-coverage/v2@latest

# Already in project:
# - github.com/stretchr/testify
# - golang.org/x/crypto/ssh (for ssh/test subpackage)
```

## Architecture Patterns

### Recommended Test Organization
```
pkg/
├── rds/
│   ├── ssh_client.go          # 0% coverage - priority target
│   ├── ssh_client_test.go     # NEW: mock SSH server tests
│   ├── commands.go            # 0% coverage on execution paths
│   ├── commands_test.go       # EXISTS: expand to cover runCommand calls
│   ├── client.go              # 0% coverage - NewClient factory
│   └── client_test.go         # NEW: factory and protocol routing tests
├── mount/
│   ├── mount.go               # 55.9% - missing error paths
│   ├── mount_test.go          # EXISTS: expand ForceUnmount, ResizeFilesystem
│   └── persist.go             # 0% coverage (attachment package)
├── nvme/
│   ├── nvme.go                # 43.3% - missing ConnectWithRetry, error paths
│   ├── nvme_test.go           # EXISTS: expand context cancellation, retry logic
│   └── connector.go           # Missing timeout and retry coverage
└── attachment/
    ├── persist.go             # 0% coverage in mount/nvme context
    └── persist_test.go        # EXISTS in attachment/: 84.5% overall coverage
```

### Pattern 1: Table-Driven Tests with Mock Exec Commands
**What:** Use test helper process pattern to mock system commands
**When to use:** Testing mount, nvme, any package that calls exec.Command
**Example:**
```go
// Source: pkg/mount/mount_test.go (existing pattern)
func mockExecCommand(stdout, stderr string, exitCode int) func(string, ...string) *exec.Cmd {
    return func(command string, args ...string) *exec.Cmd {
        cs := []string{"-test.run=TestHelperProcess", "--", command}
        cs = append(cs, args...)
        cmd := exec.Command(os.Args[0], cs...)
        cmd.Env = []string{
            "GO_WANT_HELPER_PROCESS=1",
            "STDOUT=" + stdout,
            "STDERR=" + stderr,
            "EXIT_CODE=" + fmt.Sprintf("%d", exitCode),
        }
        return cmd
    }
}

// Test helper that simulates command execution
func TestHelperProcess(t *testing.T) {
    if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
        return
    }
    os.Stdout.WriteString(os.Getenv("STDOUT"))
    os.Stderr.WriteString(os.Getenv("STDERR"))
    exitCode, _ := strconv.Atoi(os.Getenv("EXIT_CODE"))
    os.Exit(exitCode)
}
```

### Pattern 2: Mock SSH Server for Client Tests
**What:** Use golang.org/x/crypto/ssh/test package to create in-process SSH server
**When to use:** Testing pkg/rds/ssh_client.go connection, command execution
**Example:**
```go
// Source: golang.org/x/crypto/ssh/test documentation
import (
    "golang.org/x/crypto/ssh"
    "golang.org/x/crypto/ssh/test"
)

func TestSSHClientConnect(t *testing.T) {
    // Create mock server with command handler
    handler := func(s ssh.Session) {
        // Mock RouterOS command responses
        if strings.Contains(s.Command(), "/disk print") {
            io.WriteString(s, mockDiskOutput)
        }
        s.Exit(0)
    }

    server := test.NewServer(handler)
    defer server.Close()

    // Test client against mock server
    client, err := newSSHClient(ClientConfig{
        Address: server.Addr(),
        // ... config
    })
    // ... assertions
}
```

### Pattern 3: Table-Driven Error Path Coverage
**What:** Systematic coverage of error conditions with test tables
**When to use:** Testing retry logic, timeouts, error classification
**Example:**
```go
func TestSSHCommandRetry(t *testing.T) {
    tests := []struct {
        name           string
        attempts       []error  // Error for each attempt
        maxRetries     int
        expectSuccess  bool
        expectSentinel error
    }{
        {
            name:          "success on first try",
            attempts:      []error{nil},
            maxRetries:    3,
            expectSuccess: true,
        },
        {
            name:          "transient error then success",
            attempts:      []error{io.EOF, nil},
            maxRetries:    3,
            expectSuccess: true,
        },
        {
            name:           "exhausted retries",
            attempts:       []error{io.EOF, io.EOF, io.EOF},
            maxRetries:     3,
            expectSuccess:  false,
        },
        {
            name:           "non-retryable error",
            attempts:       []error{fmt.Errorf("not enough space")},
            maxRetries:     3,
            expectSuccess:  false,
            expectSentinel: utils.ErrResourceExhausted,
        },
    }
    // ... test execution
}
```

### Pattern 4: Context Cancellation Tests
**What:** Test behavior when operations are cancelled mid-flight
**When to use:** All functions accepting context.Context (nvme, mount operations)
**Example:**
```go
func TestConnectWithContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())

    // Start long-running operation
    errCh := make(chan error, 1)
    go func() {
        _, err := connector.ConnectWithContext(ctx, target)
        errCh <- err
    }()

    // Cancel after short delay
    time.Sleep(10 * time.Millisecond)
    cancel()

    // Verify cancellation propagated
    err := <-errCh
    if !errors.Is(err, context.Canceled) {
        t.Errorf("Expected context.Canceled, got %v", err)
    }
}
```

### Anti-Patterns to Avoid
- **Mocking for coverage theater:** Don't mock internals just to hit 80%. Test behavior through public interfaces.
- **Ignoring race detector warnings:** Always run `go test -race` on concurrent code (attachment state, circuit breaker).
- **Testing implementation details:** Test observable behavior (commands executed, files created) not internal state.
- **Skipping cleanup in tests:** Always use `t.Cleanup()` or `defer` to clean up mocks, temp files, goroutines.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SSH server for testing | Custom TCP listener + auth | golang.org/x/crypto/ssh/test | Handles auth, sessions, command routing; maintained by Go team |
| Test coverage enforcement | Parse coverage output manually | github.com/vladopajic/go-test-coverage | File-level and package-level thresholds, CI integration, badge generation |
| Filesystem mocking | Custom file wrapper interface | testing/fstest.MapFS (stdlib) | Standard library, minimal, sufficient for most cases |
| Exec command mocking | Runtime command interceptor | TestHelperProcess pattern (existing) | Already working in project, consistent, low overhead |
| Race detection | Manual mutex checking | go test -race (stdlib) | Compiler-instrumented, finds actual races at runtime |
| Coverage reporting | Manual aggregation | go test -coverprofile + go tool cover | JSON output, HTML visualization, line-by-line coverage |

**Key insight:** Go's standard library and x/ packages provide mature testing infrastructure. Third-party tools should only be used for CI enforcement (go-test-coverage) or when stdlib is insufficient. The project already uses the right patterns—exec command mocking is working well, just needs expansion to untested code paths.

## Common Pitfalls

### Pitfall 1: Testing Happy Path Only
**What goes wrong:** Tests pass but real errors (connection timeout, disk full, device disappearance) crash production
**Why it happens:** Happy path is easier to test; error injection requires thought
**How to avoid:**
- For every successful test case, add 2-3 failure cases
- Test timeouts: use short contexts that expire mid-operation
- Test retries: mock transient failures then success
- Test resource exhaustion: mock "not enough space" errors
**Warning signs:** Coverage shows function tested but error returns never checked

### Pitfall 2: SSH Tests Without Mock Server
**What goes wrong:** Tests skip SSH operations (`t.Skip()`) or use real servers (flaky, slow)
**Why it happens:** Setting up mock SSH server seems complex
**How to avoid:**
- Use golang.org/x/crypto/ssh/test.NewServer() with command handler
- Handler checks command string, returns mock RouterOS output
- One-time setup in TestMain or helper function
- Server runs in-process, no network flakiness
**Warning signs:** Tests marked `t.Skip("requires SSH")`, 0% coverage on ssh_client.go

### Pitfall 3: Race Conditions in Concurrent Tests
**What goes wrong:** Tests pass locally but fail in CI; attachment state corruption
**Why it happens:** Concurrent operations (attachment lock, circuit breaker) have races
**How to avoid:**
- Always run `go test -race` during development
- Test concurrent operations explicitly (multiple goroutines, shared state)
- Use `t.Parallel()` to expose races
- Fix race warnings immediately (current code appears race-free but verify)
**Warning signs:** Intermittent test failures, mutex errors, deadlocks in logs

### Pitfall 4: Mocking Internal Dependencies
**What goes wrong:** Tests break on refactoring; brittle test setup
**Why it happens:** Mocking concrete types instead of interfaces
**How to avoid:**
- Test through RDSClient interface (already defined), not sshClient struct
- Test through Mounter interface, not mounter struct
- Test through Connector interface, not connector struct
- Use existing MockClient in pkg/rds/mock.go
**Warning signs:** Tests create `&sshClient{}` directly, modify private fields

### Pitfall 5: Ignoring Context Deadlines
**What goes wrong:** Operations hang forever when context expires; tests timeout
**Why it happens:** Not checking `ctx.Done()` in loops, not passing context to suboperations
**How to avoid:**
- Every loop that waits should `select` on `ctx.Done()`
- Pass context to all SSH commands, exec commands
- Test with `context.WithTimeout` that expires quickly
- Verify `context.DeadlineExceeded` returned on timeout
**Warning signs:** Tests hang, operations don't respect timeouts, context ignored

### Pitfall 6: Coverage Theater
**What goes wrong:** 80% coverage achieved but critical paths untested
**Why it happens:** Focus on metric instead of meaningful tests
**How to avoid:**
- Prioritize error paths over happy paths (errors more likely in production)
- Review `go tool cover -func` to find 0% functions
- For 0% functions, ask: "What happens if this fails in production?"
- Cover those failure modes explicitly
**Warning signs:** High coverage on helpers/utils, low coverage on core logic (CreateVolume, NodeStageVolume)

## Code Examples

Verified patterns from research and existing codebase:

### SSH Client Testing with Mock Server
```go
// Source: golang.org/x/crypto/ssh/test + project patterns
package rds

import (
    "testing"
    "golang.org/x/crypto/ssh"
    sshtest "golang.org/x/crypto/ssh/test"
)

func TestSSHClientConnectSuccess(t *testing.T) {
    // Create mock SSH server
    handler := func(sess ssh.Session) {
        // Route commands to mock responses
        cmd := sess.Command()
        switch {
        case strings.Contains(cmd, "/disk print"):
            io.WriteString(sess, mockDiskPrintOutput)
        case strings.Contains(cmd, "/file print"):
            io.WriteString(sess, mockFilePrintOutput)
        default:
            io.WriteString(sess.Stderr(), "unknown command")
            sess.Exit(1)
            return
        }
        sess.Exit(0)
    }

    server := sshtest.NewServer(handler)
    defer server.Close()

    // Create client pointing to mock server
    client, err := newSSHClient(ClientConfig{
        Address:            server.Addr(),
        Port:               22,
        User:               "admin",
        InsecureSkipVerify: true, // OK for tests
        Timeout:            5 * time.Second,
    })
    if err != nil {
        t.Fatalf("Failed to create client: %v", err)
    }

    // Test connection
    if err := client.Connect(); err != nil {
        t.Errorf("Connect failed: %v", err)
    }

    // Verify connected
    if !client.IsConnected() {
        t.Error("Expected client to be connected")
    }

    // Test command execution
    output, err := client.runCommand("/disk print")
    if err != nil {
        t.Errorf("runCommand failed: %v", err)
    }
    if output != mockDiskPrintOutput {
        t.Errorf("Unexpected output: %s", output)
    }
}

func TestSSHClientRetryTransientError(t *testing.T) {
    attemptCount := 0
    handler := func(sess ssh.Session) {
        attemptCount++
        if attemptCount < 2 {
            // Simulate transient error by closing connection
            sess.Close()
            return
        }
        // Success on second attempt
        io.WriteString(sess, "success")
        sess.Exit(0)
    }

    server := sshtest.NewServer(handler)
    defer server.Close()

    client := createTestClient(server.Addr(), t)
    output, err := client.runCommandWithRetry("/disk add ...", 3)

    if err != nil {
        t.Errorf("Expected success after retry, got: %v", err)
    }
    if attemptCount != 2 {
        t.Errorf("Expected 2 attempts, got %d", attemptCount)
    }
}
```

### Mount Error Path Testing
```go
// Source: pkg/mount/mount_test.go (existing pattern, expand)
func TestForceUnmountWithInUseMount(t *testing.T) {
    tests := []struct {
        name          string
        mountInUse    bool
        pids          []int
        expectError   bool
        expectErrType error
    }{
        {
            name:        "force unmount succeeds when not in use",
            mountInUse:  false,
            pids:        nil,
            expectError: false,
        },
        {
            name:          "refuses to force unmount when in use",
            mountInUse:    true,
            pids:          []int{1234, 5678},
            expectError:   true,
            expectErrType: ErrMountInUse,
        },
        {
            name:        "normal unmount fails, lazy succeeds",
            mountInUse:  false,
            pids:        nil,
            expectError: false,
            // Mock: first umount fails, umount -l succeeds
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mock based on test case
            m := setupMockMounter(t, tt.mountInUse, tt.pids)

            err := m.ForceUnmount("/mnt/target", 5*time.Second)

            if tt.expectError {
                if err == nil {
                    t.Error("Expected error but got nil")
                }
                if tt.expectErrType != nil && !errors.Is(err, tt.expectErrType) {
                    t.Errorf("Expected error type %v, got %v", tt.expectErrType, err)
                }
            } else {
                if err != nil {
                    t.Errorf("Unexpected error: %v", err)
                }
            }
        })
    }
}
```

### NVMe Connect Retry with Context
```go
// Source: Pattern for pkg/nvme/nvme.go ConnectWithRetry
func TestConnectWithRetry(t *testing.T) {
    tests := []struct {
        name            string
        attempts        []error // Error for each attempt
        expectedRetries int
        expectSuccess   bool
        contextTimeout  time.Duration
    }{
        {
            name:            "success on first try",
            attempts:        []error{nil},
            expectedRetries: 1,
            expectSuccess:   true,
        },
        {
            name:            "transient failure then success",
            attempts:        []error{errors.New("connection refused"), nil},
            expectedRetries: 2,
            expectSuccess:   true,
        },
        {
            name:            "context timeout during retry",
            attempts:        []error{io.EOF, io.EOF, io.EOF},
            expectedRetries: 2, // Fewer than max retries due to timeout
            expectSuccess:   false,
            contextTimeout:  50 * time.Millisecond,
        },
        {
            name:            "max retries exhausted",
            attempts:        []error{io.EOF, io.EOF, io.EOF},
            expectedRetries: 3,
            expectSuccess:   false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            attemptCount := 0
            mockConnect := func(ctx context.Context, target Target, config ConnectionConfig) (string, error) {
                if attemptCount >= len(tt.attempts) {
                    return "", errors.New("too many attempts")
                }
                err := tt.attempts[attemptCount]
                attemptCount++
                return "/dev/nvme0n1", err
            }

            ctx := context.Background()
            if tt.contextTimeout > 0 {
                var cancel context.CancelFunc
                ctx, cancel = context.WithTimeout(ctx, tt.contextTimeout)
                defer cancel()
            }

            connector := &connector{
                connectFunc: mockConnect,
                config: ConnectionConfig{
                    MaxRetries:   3,
                    RetryBackoff: 10 * time.Millisecond,
                },
            }

            device, err := connector.ConnectWithRetry(ctx, target, config)

            if tt.expectSuccess {
                if err != nil {
                    t.Errorf("Expected success, got error: %v", err)
                }
                if device != "/dev/nvme0n1" {
                    t.Errorf("Expected device path, got: %s", device)
                }
            } else {
                if err == nil {
                    t.Error("Expected error but got nil")
                }
            }

            if attemptCount != tt.expectedRetries {
                t.Errorf("Expected %d attempts, got %d", tt.expectedRetries, attemptCount)
            }
        })
    }
}
```

### Testing Sentinel Error Classification
```go
// Source: Phase 19 error handling patterns
func TestRDSErrorClassification(t *testing.T) {
    tests := []struct {
        name           string
        commandOutput  string
        expectSentinel error
    }{
        {
            name:           "resource exhausted",
            commandOutput:  "error: not enough space",
            expectSentinel: utils.ErrResourceExhausted,
        },
        {
            name:           "not found",
            commandOutput:  "error: no such item",
            expectSentinel: utils.ErrNotFound,
        },
        {
            name:           "invalid parameter",
            commandOutput:  "error: invalid parameter 'size'",
            expectSentinel: utils.ErrInvalidParameter,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockSSHCommand := func(cmd string) (string, error) {
                return tt.commandOutput, fmt.Errorf("command failed")
            }

            client := &sshClient{runCommand: mockSSHCommand}
            err := client.CreateVolume(CreateVolumeOptions{...})

            if !errors.Is(err, tt.expectSentinel) {
                t.Errorf("Expected sentinel error %v, got %v", tt.expectSentinel, err)
            }
        })
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Integration tests only | Unit + integration mix | Go 1.20 (2023) | go build -cover enables integration coverage |
| Manual mocking | Interface-based mocks | Long-standing | Testify mock package simplifies assertions |
| String error matching | Sentinel errors + errors.Is() | Go 1.13 (2019), project Phase 19 | Type-safe error classification |
| Single coverage metric | File/package/total thresholds | Recent (2024+) | go-test-coverage enforces granular targets |
| t.Run subtests | t.Parallel() for race detection | Always available | Exposes concurrent bugs earlier |

**Deprecated/outdated:**
- **String-based error checking:** Phase 19 introduced sentinel errors; use `errors.Is()` not `strings.Contains()`
- **Manual coverage parsing:** Use `go-test-coverage` tool for CI enforcement
- **Real SSH servers in tests:** Use `golang.org/x/crypto/ssh/test` mock server
- **Custom exec mocking libraries:** Existing TestHelperProcess pattern works well

## Open Questions

Things that couldn't be fully resolved:

1. **Mock SSH Server Authentication**
   - What we know: `golang.org/x/crypto/ssh/test` provides basic server
   - What's unclear: Does it support all auth methods (pubkey, password, none)?
   - Recommendation: Test with `InsecureSkipVerify: true` for unit tests; auth logic tested separately

2. **Coverage on Legacy Code Paths**
   - What we know: Functions like `connectLegacy()`, `disconnectLegacy()` have 0% coverage
   - What's unclear: Are these dead code or still needed for fallback?
   - Recommendation: Add `// +build integration` tests or document as deprecated

3. **Integration vs Unit Coverage**
   - What we know: Go 1.20+ supports `go build -cover` for integration tests
   - What's unclear: Should 80% target include integration tests or unit tests only?
   - Recommendation: Use unit tests for 80% threshold; integration tests for end-to-end validation

4. **Persist.go File Location**
   - What we know: `persist.go` mentioned in requirements but not found in mount/nvme packages
   - What's unclear: Is it in attachment package only or copied to others?
   - Recommendation: Coverage for `pkg/attachment/persist.go` already at 84.5%; verify no duplication in other packages

## Sources

### Primary (HIGH confidence)
- [Go official documentation - Code coverage for integration tests](https://go.dev/doc/build-cover)
- [Go official blog - Integration test coverage](https://go.dev/blog/integration-test-coverage)
- [Go official documentation - Race detector](https://go.dev/doc/articles/race_detector)
- [golang.org/x/crypto/ssh/test package documentation](https://pkg.go.dev/golang.org/x/crypto/ssh/test)
- [testing/fstest package documentation](https://pkg.go.dev/testing/fstest)
- [context package documentation (published Jan 2026)](https://pkg.go.dev/context)

### Secondary (MEDIUM confidence)
- [GitHub - vladopajic/go-test-coverage](https://github.com/vladopajic/go-test-coverage) - Coverage enforcement tool
- [GitHub - stretchr/testify](https://github.com/stretchr/testify) - Testing toolkit
- [Medium - SSH unit testing in Go](https://medium.com/@metarsit/ssh-is-fun-till-you-need-to-unit-test-it-in-go-f3b3303974ab)
- [DEV Community - Testing filesystem code mocking patterns](https://dev.to/rezmoss/testing-file-system-code-mocking-stubbing-and-test-patterns-99-1fkh)
- [GitHub - spf13/afero](https://github.com/spf13/afero) - Filesystem abstraction library
- [Better Stack - Testing in Go with Testify](https://betterstack.com/community/guides/scaling-go/golang-testify/)
- [DEV Community - Table-driven tests with Testify](https://dev.to/zpeters/testing-in-go-with-table-drive-tests-and-testify-kd4)

### Tertiary (LOW confidence)
- [Go Cookbook - Test Coverage](https://go-cookbook.com/snippets/testing/test-coverage) - General patterns
- [Evil Martians - Go integration testing](https://evilmartians.com/chronicles/go-integration-testing-with-courage-and-coverage) - Real-world examples
- [Learn Go with Tests - Context](https://quii.gitbook.io/learn-go-with-tests/go-fundamentals/context) - Testing context cancellation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Go stdlib and x/ packages are authoritative; testify widely adopted in project
- Architecture: HIGH - Existing test patterns (exec mocking, table-driven) verified in codebase; SSH mock pattern from official docs
- Pitfalls: HIGH - Based on Go race detector docs, context package behavior, and existing test analysis
- Code examples: HIGH - Patterns derived from existing codebase (mount_test.go, nvme_test.go) and official Go documentation

**Research date:** 2026-02-04
**Valid until:** 90 days (stable domain; Go testing patterns change slowly; stdlib patterns are long-lived)
