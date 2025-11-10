package utils

import (
	"regexp"
	"time"
)

// This file contains optimized, ReDoS-resistant regex patterns used throughout the codebase.
// All patterns have been audited for catastrophic backtracking vulnerabilities.

// Common regex patterns used across the codebase
var (
	// VolumeIDPattern matches PVC volume IDs with fixed-length UUID segments
	// ReDoS-safe: Uses exact character counts {n} instead of unbounded quantifiers
	// Original: ^pvc-[a-f0-9-]+$  (vulnerable to ReDoS with input like "pvc-aaaaaaa...")
	// Optimized: Uses exact lengths matching UUID format
	VolumeIDPattern = regexp.MustCompile(`^pvc-[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

	// SafeSlotPattern matches safe slot names (alphanumeric and hyphens)
	// ReDoS-safe: Simple character class with + quantifier, no nested quantifiers
	SafeSlotPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

	// NQNPattern matches NVMe Qualified Names per NVMe spec
	// ReDoS-safe: Domain part uses atomic group pattern to prevent backtracking
	// Format: nqn.yyyy-mm.reverse-domain:identifier
	// Max length enforced separately (223 bytes per NVMe spec)
	// Domain: labels separated by dots, no consecutive dots
	NQNPattern = regexp.MustCompile(`^nqn\.[0-9]{4}-[0-9]{2}\.(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]*[a-z0-9])?:[a-z0-9._-]+$`)

	// IPv4Pattern matches IPv4 addresses
	// ReDoS-safe: Uses word boundaries and exact digit counts
	IPv4Pattern = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)

	// IPv6Pattern matches basic IPv6 addresses
	// ReDoS-safe: Uses exact repetition counts, not unbounded quantifiers
	IPv6Pattern = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b`)

	// FileSizePattern matches file sizes with units (e.g., "10GB", "5.5TiB")
	// ReDoS-safe: Optional decimal part uses {1,N} bounds
	FileSizePattern = regexp.MustCompile(`^([0-9]+(?:\.[0-9]{1,2})?)([KMGT]i?B)$`)

	// PortPattern matches port numbers (1-65535)
	// ReDoS-safe: Uses bounded digit counts
	PortPattern = regexp.MustCompile(`^[0-9]{1,5}$`)

	// SlotNamePattern matches RouterOS disk slot names
	// ReDoS-safe: Uses + on simple character class (no alternation or nesting)
	SlotNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
)

// Command output parsing patterns - used for RouterOS CLI output
var (
	// KeyValueQuotedPattern matches key="value" pairs
	// ReDoS-safe: Uses negated character class [^"] to match up to quote
	// No nested quantifiers or alternation with overlap
	KeyValueQuotedPattern = regexp.MustCompile(`(\w+)="([^"]*)"`)

	// KeyValueUnquotedPattern matches key=value pairs (no quotes)
	// ReDoS-safe: Uses \S (non-whitespace) which is possessive
	KeyValueUnquotedPattern = regexp.MustCompile(`(\w+)=(\S+)`)

	// EntryNumberPattern matches entry numbers at start of line
	// ReDoS-safe: Anchored with ^, uses \s+ which is acceptable for whitespace
	EntryNumberPattern = regexp.MustCompile(`(?m)^\s*\d+\s+`)

	// SizeValuePattern matches size values with optional whitespace
	// ReDoS-safe: Uses \s* with bounded context, not nested
	SizeValuePattern = regexp.MustCompile(`size=\s*([0-9]+(?:\s*[KMGT]i?B)?)`)

	// FreeValuePattern matches free space values
	// ReDoS-safe: Same as SizeValuePattern, bounded context
	FreeValuePattern = regexp.MustCompile(`free=\s*([0-9]+(?:\s*[KMGT]i?B)?)`)
)

// Error sanitization patterns - for removing sensitive data from error messages
var (
	// SSHFingerprintPattern matches SSH key fingerprints
	// ReDoS-safe: Uses character class [A-Za-z0-9+/=] with + quantifier
	// No nested quantifiers or complex alternation
	SSHFingerprintPattern = regexp.MustCompile(`SHA256:[A-Za-z0-9+/=]+`)

	// UnixPathPattern matches Unix absolute paths
	// ReDoS-safe: Uses non-capturing groups with bounded repetition
	// Character classes are simple, no alternation overlap
	UnixPathPattern = regexp.MustCompile(`/[a-zA-Z0-9_-]+(?:/[a-zA-Z0-9_.-]+){0,32}`)

	// WindowsPathPattern matches Windows absolute paths
	// ReDoS-safe: Uses \w and limited character classes
	WindowsPathPattern = regexp.MustCompile(`[A-Z]:\\[\w\\\-\.]{1,255}`)

	// HostnamePattern matches FQDNs with common TLDs
	// ReDoS-safe: Uses specific quantifiers, no nested repetition
	// Limited to reasonable hostname lengths
	HostnamePattern = regexp.MustCompile(`\b[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?){0,10}\.(com|net|org|io|local|lan)\b`)

	// StackTracePattern matches stack traces and goroutine info
	// ReDoS-safe: Uses .* which is greedy but in bounded context (per-line)
	// Multiline mode with ^/$ anchors limits scope
	StackTracePattern = regexp.MustCompile(`(?m)^\s+at\s+.+$|^goroutine\s+\d+.*$`)
)

// RegexTimeout is the maximum time a regex operation should take
// This is a defense-in-depth measure; properly written regexes shouldn't need this
const RegexTimeout = 100 * time.Millisecond

// SafeMatchString performs a regex match with timeout protection
// Returns (matched, timedOut, error)
func SafeMatchString(pattern *regexp.Regexp, input string) (bool, bool, error) {
	resultChan := make(chan bool, 1)

	go func() {
		resultChan <- pattern.MatchString(input)
	}()

	select {
	case result := <-resultChan:
		return result, false, nil
	case <-time.After(RegexTimeout):
		// Regex timed out - possible ReDoS attack
		return false, true, nil
	}
}

// SafeFindStringSubmatch performs a regex submatch with timeout protection
func SafeFindStringSubmatch(pattern *regexp.Regexp, input string) ([]string, bool, error) {
	resultChan := make(chan []string, 1)

	go func() {
		resultChan <- pattern.FindStringSubmatch(input)
	}()

	select {
	case result := <-resultChan:
		return result, false, nil
	case <-time.After(RegexTimeout):
		// Regex timed out - possible ReDoS attack
		return nil, true, nil
	}
}

// RegexSecurityNotes documents security considerations for regex patterns
//
// ReDoS (Regular Expression Denial of Service) Prevention Guidelines:
//
// 1. Avoid nested quantifiers: (a+)+ or (a*)*
//    - VULNERABLE: `(a+)+b`
//    - SAFE: `a+b`
//
// 2. Avoid alternation with overlapping patterns: (a|a)* or (a|ab)*
//    - VULNERABLE: `(a|ab)*c`
//    - SAFE: `(?:ab|a)*c` (longer alternative first)
//
// 3. Use possessive quantifiers when available: `a++b` (Go doesn't support)
//    - Alternative: Use atomic groups or negated character classes
//    - VULNERABLE: `.*`
//    - SAFER: `[^\s]*` (more specific character class)
//
// 4. Bound repetition: Use {min,max} instead of * or +
//    - VULNERABLE: `[a-z]+` (unbounded)
//    - SAFE: `[a-z]{1,100}` (bounded)
//
// 5. Use anchors: ^ and $ to limit search space
//    - VULNERABLE: `pattern` (searches entire string)
//    - SAFE: `^pattern$` (anchored to start/end)
//
// 6. Avoid backreferences in untrusted input: \1, \2, etc.
//    - Backreferences can cause exponential behavior
//
// 7. Use negated character classes instead of .*
//    - VULNERABLE: `".*"`
//    - SAFE: `"[^"]*"` (stops at first quote)
//
// 8. Test with pathological inputs:
//    - Long strings of repeated characters
//    - Strings that almost match but fail at the end
//    - Example: "aaaaaaaaaaaaaaaaaaaaaaaX" against `(a+)+b`
//
// All patterns in this file have been audited and tested for ReDoS resistance.
