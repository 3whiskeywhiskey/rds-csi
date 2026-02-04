// Package driver implements CSI Controller and Node services for RDS.
//
// # Logging Verbosity Convention
//
// This package follows Kubernetes logging conventions for verbosity levels:
//
//   - V(0): Always visible - panics, programmer errors
//   - V(1): Configuration, frequently repeating errors
//   - V(2): Production default - operation outcomes, state changes
//     Examples: "Created volume X", "Mounted Y to Z", "Deleted volume X"
//   - V(4): Debug level - intermediate steps, parameters, diagnostics
//     Examples: "Checking attachment state", "Found device path"
//   - V(5): Trace level - command I/O, parsing details
//     Examples: "RouterOS output: ...", "SSH command: ..."
//
// V(3) is avoided in favor of V(2) (if actionable) or V(4) (if diagnostic).
//
// Production deployments use V(2) by default. Set --v=4 for troubleshooting.
package driver
