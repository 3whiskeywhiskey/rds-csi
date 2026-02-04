---
phase: 19
plan: 03
subsystem: documentation
tags: [error-handling, conventions, documentation]
requires:
  - "19-02: Sentinel errors and helpers implemented"
provides:
  - "Comprehensive error handling documentation"
  - "Clear guidance on %w vs %v usage"
  - "gRPC boundary conversion patterns"
affects:
  - "Future contributors understand error patterns"
  - "Code reviews can reference documented standards"
decisions:
  - id: ERR-03
    title: "Error patterns documented in CONVENTIONS.md"
    rationale: "Comprehensive documentation ensures consistent error handling across all contributors"
    date: 2026-02-04
tech-stack:
  added: []
  patterns:
    - "Error wrapping with %w for chain preservation"
    - "Layered context pattern (one context per layer)"
    - "gRPC boundary conversion at CSI service layer"
key-files:
  created: []
  modified:
    - path: ".planning/codebase/CONVENTIONS.md"
      lines-added: 167
      lines-removed: 14
      significance: "Expanded Error Handling section from ~28 to 183 lines"
metrics:
  duration: "90s"
  completed: "2026-02-04"
---

# Phase 19 Plan 03: Error Handling Documentation Summary

**One-liner:** Comprehensive error handling documentation in CONVENTIONS.md covering %w/%v usage, sentinel errors, layered context, gRPC boundaries, and common mistakes

## What Was Built

Expanded the Error Handling section in CONVENTIONS.md from ~28 lines to 183 lines with comprehensive coverage of all error handling patterns used in the RDS CSI driver codebase.

### Key Documentation Added

**Quick Reference:**
- Summarized core error handling rules
- %w for wrapping errors, %v for formatting values
- Context addition at each layer
- gRPC conversion at boundaries

**Error Wrapping with %w:**
- Explained Go 1.13+ error wrapping
- Clear examples of correct vs incorrect usage
- When to use %v (non-error values only)

**Sentinel Errors:**
- Documented all 10 sentinel errors in pkg/utils/errors.go
- Showed wrapping pattern with context
- Demonstrated error checking with errors.Is()

**Layered Context Pattern:**
- Explained one-context-per-layer principle
- Showed bottom/middle/top layer examples
- Prevents duplicate information in error chains

**gRPC Boundary Conversion:**
- CSI service layer uses status.Error/Errorf
- Internal packages use fmt.Errorf with %w
- Clear separation of concerns

**Error Context Requirements:**
- Every error needs: operation, resource ID, reason
- Documented helper functions (WrapVolumeError, etc.)

**Common Mistakes:**
- Using %v for errors (breaks chain)
- Double-wrapping with same message
- Wrapping gRPC status errors
- Silent error handling

**Error Inspection:**
- Type-safe checking with errors.Is()
- Error extraction with errors.As()
- Context timeout detection

## Deviations from Plan

None - plan executed exactly as written.

## Technical Decisions

**Decision ERR-03: Document error patterns in CONVENTIONS.md**

- **Context:** Existing codebase already has sophisticated error handling (96% compliant with %w usage)
- **Decision:** Comprehensive documentation of existing patterns for future contributors
- **Rationale:**
  - Prevents regression to %v usage
  - Provides clear guidance for code reviews
  - Explains layered context pattern to avoid duplication
  - Documents gRPC boundary conversion rules
- **Impact:** Contributors can reference documented standards during development

**Section Organization:**
- Quick Reference first for fast lookup
- Detailed subsections for learning
- Common Mistakes for code reviews
- Error Inspection for debugging

## Testing Evidence

**Verification:**
```bash
# Confirm section expansion
$ sed -n '/^## Error Handling/,/^## Logging/p' .planning/codebase/CONVENTIONS.md | wc -l
183

# Verify all subsections present
$ sed -n '/^## Error Handling/,/^## Logging/p' .planning/codebase/CONVENTIONS.md | grep "^###"
### Quick Reference
### Error Wrapping with %w
### Sentinel Errors
### Layered Context Pattern
### gRPC Boundary Conversion
### Error Context Requirements
### Common Mistakes
### Error Inspection
```

## Integration Points

**With Phase 19-02:**
- Documents sentinel errors implemented in 19-02
- Shows usage of WrapVolumeError/WrapNodeError helpers from 19-02

**With Phase 19-04 (Future):**
- Provides documentation foundation for linter configuration
- Patterns can be enforced via errorlint, errcheck rules

## Operational Impact

**For Contributors:**
- Clear guidance on %w vs %v usage
- Examples prevent common mistakes
- Code reviews can reference specific sections

**For Maintainers:**
- Documented standards for code reviews
- Prevents drift from established patterns
- Easy to onboard new contributors

## Next Phase Readiness

**Phase 19-04 (Linter Configuration):**
- Documentation complete, ready for automated enforcement
- Patterns are clearly defined for linter rules

**Blockers:** None

**Risks:** None

## Lessons Learned

**What Worked Well:**
- Existing error infrastructure was already excellent (96% compliant)
- Documentation captures patterns already in practice
- Clear examples make patterns easy to follow

**What Could Be Improved:**
- Could add diagrams for error flow through layers
- Could include more examples from actual codebase

**Recommendations for Future:**
- Keep CONVENTIONS.md up-to-date as patterns evolve
- Reference this section in PR templates
- Consider automated link checking in CI

## Commits

- **7da16e4**: docs(19-03): expand error handling documentation in CONVENTIONS.md
  - Added 8 subsections with comprehensive coverage
  - 167 lines added, 14 lines removed
  - Covers %w/%v, sentinels, layered context, gRPC, mistakes, inspection

## Success Metrics

- ✅ Error Handling section expanded from ~28 to 183 lines (554% increase)
- ✅ All 8 required topics covered (Quick Reference, %w/%v, Sentinels, Layered Context, gRPC, Context Requirements, Common Mistakes, Inspection)
- ✅ Clear guidance on %w vs %v usage with examples
- ✅ gRPC boundary conversion documented
- ✅ Common mistakes called out with corrections
- ✅ ERR-03 complete: Error handling patterns documented

**Quality Indicators:**
- Documentation structure: Quick Reference → Details → Mistakes → Inspection
- Code examples: 15+ code snippets showing correct vs incorrect patterns
- Coverage: All sentinel errors from pkg/utils/errors.go documented
- Usability: Easy to scan, easy to reference in code reviews
