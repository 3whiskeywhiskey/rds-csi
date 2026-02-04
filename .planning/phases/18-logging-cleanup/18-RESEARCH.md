# Phase 18: Logging Cleanup - Research

**Researched:** 2026-02-04
**Domain:** Go structured logging, klog verbosity conventions, code consolidation patterns
**Confidence:** HIGH

## Summary

This phase addresses production log noise through systematic verbosity rationalization. The codebase currently has 469 klog statements (164 non-verbosity, 134 at V(2), 89 at V(4), 53 at V(3), 25 at V(5)). Key problems identified:

1. **Security logger bloat**: 540 lines with 18 methods containing repetitive switch/case patterns for outcome-based event types
2. **Command output spam**: RouterOS command output logged at V(5) (debug) but needs audit of where verbose output appears at info level
3. **DeleteVolume verbosity**: Currently uses 3-4 log statements (V(2), V(3)) per operation with redundant status messages

The Kubernetes logging convention establishes V(2) as "useful steady state information" - production default. V(3)+ is for debugging. The phase will move diagnostic details to appropriate verbosity levels while consolidating repetitive logger methods into configurable helpers.

**Primary recommendation:** Use table-driven patterns for security logger consolidation, move command details to V(4)+, reduce DeleteVolume to single outcome log at V(2) with details at V(4).

## Standard Stack

The established logging approach for Kubernetes CSI drivers:

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| k8s.io/klog/v2 | v2.x | Structured logging with verbosity levels | Official Kubernetes logging standard, used by all core components and CSI drivers |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| log/slog | Go 1.21+ | Standard library structured logging | Alternative for new Go projects, but klog is required for Kubernetes integration |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| klog | log/slog | Modern Go standard but incompatible with Kubernetes ecosystem conventions |
| klog | zerolog/zap | Higher performance but adds dependency and breaks Kubernetes conventions |

**Installation:**
```bash
# Already in use - no additional dependencies needed
go get k8s.io/klog/v2
```

**Note:** Kubernetes is transitioning toward OpenTelemetry for observability, but klog remains the standard for CSI drivers as of 2026.

## Architecture Patterns

### Kubernetes Logging Convention (Official)

Verbosity level semantics from [Kubernetes Logging Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md):

- **V(0) / InfoS**: Always visible to operators - programmer errors, panics, CLI handling
- **V(1)**: Reasonable default - configuration, frequently repeating errors
- **V(2)**: **Production default** - useful steady state, HTTP requests, system state changes, controller transitions
- **V(3)**: Extended information about changes - detailed state modifications
- **V(4)**: Debug level - thorough instrumentation in complex code
- **V(5)**: Trace level - context for errors/warnings, troubleshooting details

**Key principle:** Lower numbers = more important. V(2) is the practical default for production. QE/dev environments use V(3) or V(4).

### Pattern 1: Verbosity Rationalization

**What:** Systematic audit of log statements to place them at semantically correct levels

**Current state:**
```go
// BAD: Command output at V(5) but other verbose diagnostics may be at V(2)
klog.V(5).Infof("Command output: %s", output)

// BAD: Multiple status messages for single operation
klog.V(2).Infof("Deleting volume %s", slot)
klog.V(3).Infof("Volume %s has backing file: %s", slot, filePath)
klog.V(3).Infof("Successfully removed disk slot for volume %s", slot)
klog.V(3).Infof("Successfully deleted backing file %s", filePath, slot)
klog.V(2).Infof("Successfully deleted volume %s", slot)
```

**Target state:**
```go
// GOOD: Outcome at V(2), diagnostic details at V(4)
klog.V(2).Infof("Deleted volume %s", volumeID)
klog.V(4).Infof("Deleted volume %s (file=%s size=%d)", volumeID, filePath, size)

// GOOD: Command output at V(5) (trace level for troubleshooting)
klog.V(5).Infof("RouterOS command: %s", command)
klog.V(5).Infof("RouterOS output: %s", output)
```

**Info level mapping:**
- **Operation outcome only**: V(2) - "Created volume X", "Deleted volume Y"
- **Operation details**: V(4) - Parameters, paths, command syntax
- **Command traces**: V(5) - Full RouterOS output, parsing details

### Pattern 2: Helper Function Consolidation

**What:** Replace repetitive methods with configurable helper that reduces code by 90%

**Current pattern (repetitive):**
```go
// 11 nearly-identical methods, each 30-40 lines
func (l *Logger) LogVolumeCreate(...) {
    var eventType EventType
    var severity EventSeverity
    var message string
    switch outcome {
    case OutcomeSuccess: eventType = EventVolumeCreateSuccess; severity = SeverityInfo; message = "..."
    case OutcomeFailure: eventType = EventVolumeCreateFailure; severity = SeverityError; message = "..."
    default: eventType = EventVolumeCreateRequest; severity = SeverityInfo; message = "..."
    }
    event := NewSecurityEvent(...).WithVolume(...).WithOutcome(...)
    l.LogEvent(event)
}
```

**Target pattern (table-driven):**
```go
// Single configurable helper (~30 lines total)
type OperationLogConfig struct {
    BaseEventType EventType  // e.g., EventVolumeCreate
    SuccessMsg    string
    FailureMsg    string
    RequestMsg    string
}

func (l *Logger) LogOperation(outcome EventOutcome, config OperationLogConfig, fields ...EventField) {
    eventType, severity, message := resolveOutcomeMapping(outcome, config)
    event := NewSecurityEvent(eventType, CategoryVolumeOperation, severity, message)
    for _, field := range fields {
        field.Apply(event)
    }
    l.LogEvent(event)
}

// Usage:
l.LogOperation(security.OutcomeSuccess, volumeCreateConfig,
    VolumeField(volumeID), DurationField(duration))
```

**Sources:**
- [Functional Options Pattern in Go](https://matheuspolitano.medium.com/unlocking-the-power-of-functional-options-pattern-in-go-087478f57be9)
- [Table-Driven Tests](https://github.com/Pungyeon/clean-go-article)

### Pattern 3: Severity Mapping Table

**What:** Replace switch statements with table-driven severity resolution

**Current (60 lines across methods):**
```go
switch outcome {
case OutcomeSuccess:
    eventType = EventVolumeCreateSuccess
    severity = SeverityInfo
    message = "Volume created successfully"
case OutcomeFailure:
    eventType = EventVolumeCreateFailure
    severity = SeverityError
    message = "Volume creation failed"
default:
    eventType = EventVolumeCreateRequest
    severity = SeverityInfo
    message = "Volume creation requested"
}
```

**Target (15 lines, reusable):**
```go
type OutcomeMappingTable map[EventOutcome]struct {
    TypeSuffix string
    Severity   EventSeverity
    Message    string
}

var standardOutcomeMapping = OutcomeMappingTable{
    OutcomeSuccess: {"Success", SeverityInfo, "%s successful"},
    OutcomeFailure: {"Failure", SeverityError, "%s failed"},
    OutcomeUnknown: {"Request", SeverityInfo, "%s requested"},
}

func resolveOutcomeMapping(outcome EventOutcome, config OperationLogConfig) (EventType, EventSeverity, string) {
    mapping := standardOutcomeMapping[outcome]
    return config.BaseEventType + mapping.TypeSuffix, mapping.Severity, fmt.Sprintf(mapping.Message, config.Operation)
}
```

### Anti-Patterns to Avoid

- **Logging everything at V(2):** Results in noisy production logs. V(2) should only contain actionable steady-state information.
- **Duplicate outcome messages:** If DeleteVolume logs "Deleting..." at start and "Successfully deleted..." at end, the start message is redundant (operation outcome is sufficient).
- **Command output at info level:** RouterOS command responses are diagnostic details, not operational outcomes. These belong at V(4)+ for troubleshooting.
- **Multiple severity levels for same event:** If an operation logs at V(2) and V(3), consolidate to single appropriate level with structured fields.

## Don't Hand-Roll

Problems that look simple but have existing solutions:

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Log level filtering | Custom verbosity logic | klog.V(level).Enabled() | klog already provides efficient verbosity checks with lazy evaluation |
| Structured logging | String concatenation | klog.InfoS with key-value pairs | Better for machine parsing, log aggregation systems |
| Outcome-based logging | If/else chains | Table-driven mapping | Reduces code duplication, easier to maintain and extend |
| Log formatting | Custom formatters | klog's structured logging | Consistent with Kubernetes ecosystem, works with kubectl logs --prefix |

**Key insight:** klog provides lazy evaluation (guards expensive operations) and structured logging primitives. Custom solutions lose ecosystem compatibility and performance optimizations.

## Common Pitfalls

### Pitfall 1: Over-Logging at Low Verbosity

**What goes wrong:** Production logs (V(2)) become too verbose, making it hard to identify important events

**Why it happens:** Developers log every step thinking "this might be useful" without considering operator burden

**How to avoid:**
- Info level (V(0-2)) = "What happened?" (outcomes, state changes)
- Debug level (V(4-5)) = "How did it happen?" (parameters, steps, command details)
- Ask: "Would an operator need to see this in steady state?"

**Warning signs:**
- More than 2-3 log lines per operation at V(2)
- Command output or parsing details at V(2-3)
- Log volume grows linearly with operation count

### Pitfall 2: Losing Debug Information

**What goes wrong:** Moving too much to high verbosity levels makes troubleshooting difficult

**Why it happens:** Over-correction after realizing logs are too verbose

**How to avoid:**
- Keep structured outcome logs at V(2) with essential context (volumeID, error)
- Move intermediate steps to V(4) but include them
- Test troubleshooting scenarios at V(4) to verify sufficient information

**Warning signs:**
- Can't diagnose failures without adding more logging
- V(4) logs don't show operation flow
- Missing key parameters in debug output

### Pitfall 3: Helper Function Over-Abstraction

**What goes wrong:** Helper becomes more complex than the code it replaces

**Why it happens:** Trying to consolidate code that has legitimate variations

**How to avoid:**
- Target 80% case consolidation (11 similar methods → 1 helper + 2 special cases is fine)
- Keep helper focused on single pattern (outcome-based operation logging)
- If helper requires >3 parameters, reconsider if abstraction is appropriate

**Warning signs:**
- Helper has more branches than original code
- Need to pass function callbacks or complex configs
- Callers need to understand helper internals

### Pitfall 4: Incorrect Verbosity Semantics

**What goes wrong:** Using verbosity levels inconsistently with Kubernetes conventions

**Why it happens:** Not understanding standard semantic meaning of V() levels

**How to avoid:**
- V(2) = steady state outcomes (always enabled in production)
- V(3) = extended state changes (rarely used, consider V(2) or V(4))
- V(4) = debug instrumentation (development/troubleshooting)
- V(5) = trace details (command I/O, parsing internals)

**Warning signs:**
- V(3) being used extensively (usually means V(2) or V(4) is more appropriate)
- V(4) contains operation outcomes (belongs at V(2))
- V(2) contains command syntax (belongs at V(4-5))

## Code Examples

Verified patterns from official sources:

### klog Verbosity Guard

```go
// Source: https://pkg.go.dev/k8s.io/klog/v2
// Efficient: String formatting only happens if V(4) enabled
if klog.V(4).Enabled() {
    klog.V(4).Infof("Processing volume %s: path=%s size=%d nqn=%s",
        volumeID, filePath, sizeBytes, nqn)
}

// More concise (klog handles guard internally)
klog.V(4).Infof("Processing volume %s: path=%s size=%d nqn=%s",
    volumeID, filePath, sizeBytes, nqn)
```

### Structured Logging (klog.InfoS)

```go
// Source: https://pkg.go.dev/k8s.io/klog/v2
// Preferred over string concatenation - machine parseable
klog.InfoS("Created volume",
    "volumeID", volumeID,
    "size", sizeBytes,
    "path", filePath,
    "duration", duration.Milliseconds())
```

### Table-Driven Configuration

```go
// Source: Clean Go Article patterns
var operationConfigs = map[string]OperationLogConfig{
    "CreateVolume": {
        BaseEventType: EventVolumeCreate,
        SuccessMsg:    "Volume created successfully",
        FailureMsg:    "Volume creation failed",
        RequestMsg:    "Volume creation requested",
    },
    "DeleteVolume": {
        BaseEventType: EventVolumeDelete,
        SuccessMsg:    "Volume deleted successfully",
        FailureMsg:    "Volume deletion failed",
        RequestMsg:    "Volume deletion requested",
    },
}

// Usage
config := operationConfigs["CreateVolume"]
l.LogOperation(outcome, config, VolumeField(volumeID), DurationField(duration))
```

### Helper with Functional Options

```go
// Source: Functional Options Pattern
type EventField func(*SecurityEvent)

func VolumeField(volumeID string) EventField {
    return func(e *SecurityEvent) { e.VolumeID = volumeID }
}

func DurationField(d time.Duration) EventField {
    return func(e *SecurityEvent) { e.Duration = d }
}

func ErrorField(err error) EventField {
    return func(e *SecurityEvent) {
        if err != nil {
            e.Error = err.Error()
        }
    }
}

// Flexible, composable usage
l.LogOperation(outcome, config,
    VolumeField(volumeID),
    DurationField(duration),
    ErrorField(err))
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| String concatenation logging | klog.InfoS with structured key-value | klog v2.0+ (2020) | Better for log aggregation, Prometheus integration |
| Fixed verbosity levels | Contextual logging with logger injection | KEP-3077 (2022-2023) | Better testing, per-operation verbosity control |
| glog | klog v2 | Kubernetes 1.20+ | Active maintenance, structured logging support |
| klog-specific flags | Standard logging configuration | Kubernetes 1.27+ (2023) | Transitioning toward OpenTelemetry |

**Deprecated/outdated:**
- **glog**: Original Google logging library, replaced by klog (Kubernetes fork with structured logging)
- **klog v1**: Superseded by v2 with structured logging support (InfoS, ErrorS)
- **String concatenation**: Replaced by structured key-value pairs for machine parseability

**Current standard (2026):**
- klog v2 with structured logging (InfoS/ErrorS) for new code
- V() levels for verbosity control remain standard practice
- OpenTelemetry integration emerging but klog still required for CSI drivers

## Open Questions

Things that couldn't be fully resolved:

1. **Security audit vs production verbosity**
   - What we know: Security logger uses V(2) for info-level events
   - What's unclear: Should security audit trail be at V(2) (visible in production) or V(3-4) (debug only)?
   - Recommendation: Keep security events at V(2) - audit trail should be visible in production. Move verbose event details to V(4).

2. **RouterOS command output currently at V(5)**
   - What we know: Command output already at trace level (V(5))
   - What's unclear: Are there other places where command output appears at lower verbosity?
   - Recommendation: Audit all klog statements in pkg/rds/ to verify command output is consistently at V(4-5).

3. **Structured logging adoption**
   - What we know: klog.InfoS is recommended for new code
   - What's unclear: Should this phase migrate existing klog.V().Info to klog.InfoS?
   - Recommendation: Out of scope - this phase focuses on verbosity rationalization. Structured logging migration should be separate phase.

## Sources

### Primary (HIGH confidence)
- [Kubernetes Logging Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md) - Official verbosity semantics
- [klog v2 package documentation](https://pkg.go.dev/k8s.io/klog/v2) - API reference and usage patterns
- [klog GitHub repository](https://github.com/kubernetes/klog) - Implementation details and examples

### Secondary (MEDIUM confidence)
- [Structured Logging in Kubernetes with Klog](https://layer5.io/blog/kubernetes/structured-logging-in-kubernetes-with-klog) - Best practices guide
- [CSI Driver Troubleshooting Guide](https://github.com/kubernetes-sigs/vsphere-csi-driver/blob/master/docs/book/troubleshooting.md) - Production vs debug logging
- [Functional Options Pattern](https://matheuspolitano.medium.com/unlocking-the-power-of-functional-options-pattern-in-go-087478f57be9) - Helper consolidation pattern
- [Clean Go Article](https://github.com/Pungyeon/clean-go-article) - Table-driven patterns
- [How to Implement Structured Logging in Go (2026)](https://oneuptime.com/blog/post/2026-01-23-go-structured-logging/view) - Current Go logging practices

### Tertiary (LOW confidence)
- None - all findings verified with official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - klog is documented Kubernetes standard for CSI drivers
- Architecture: HIGH - Verbosity semantics defined in official Kubernetes conventions
- Pitfalls: MEDIUM-HIGH - Based on common patterns but some are experiential rather than documented

**Research date:** 2026-02-04
**Valid until:** ~90 days (logging conventions are stable, klog v2 API is mature)

**Codebase analysis:**
- Total klog statements: 469 (164 non-verbosity, 305 with V() levels)
- Security logger: 540 lines, 18 methods (11 operation-specific, 7 utility)
- Verbosity distribution: V(2)=134, V(4)=89, V(3)=53, V(5)=25
- DeleteVolume logging: 3-4 statements per operation across V(2) and V(3)

**Key metrics for success criteria:**
- Security logger target: <50 lines (current: 540) = 90% reduction
- DeleteVolume target: ≤2 statements per operation (current: 3-4) = 33-50% reduction
