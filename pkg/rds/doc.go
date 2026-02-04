// Package rds provides SSH client and RouterOS command wrappers for RDS management.
//
// # Logging Verbosity Convention
//
// This package follows Kubernetes logging conventions for verbosity levels:
//
//   - V(0): Always visible - connection failures, critical errors
//   - V(2): Production default - operation outcomes
//     Examples: "Created volume X", "Deleted volume Y", "Resized volume Z"
//   - V(4): Debug level - intermediate steps, command parameters
//     Examples: "Volume already exists", "Checking disk slot"
//   - V(5): Trace level - RouterOS command syntax, raw output
//     Examples: "Executing: /disk add...", "RouterOS response: ..."
//
// V(3) is avoided in favor of V(2) (if actionable) or V(4) (if diagnostic).
//
// Production deployments use V(2) by default. Set --v=4 for troubleshooting.
package rds
