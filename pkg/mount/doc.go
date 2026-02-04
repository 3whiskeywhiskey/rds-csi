// Package mount provides filesystem mount/unmount operations for CSI node service.
//
// # Logging Verbosity Convention
//
// This package follows Kubernetes logging conventions for verbosity levels:
//
//   - V(0): Always visible - programmer errors, panics
//   - V(2): Production default - operation outcomes, state changes
//     Examples: "Mounted /dev/nvme0n1 to /var/lib/kubelet/...", "Unmounted /path"
//   - V(4): Debug level - intermediate steps, parameters, diagnostics
//     Examples: "Checking if path is mounted", "Retrying mount (attempt 2/3)"
//   - V(5): Trace level - command output, parsing details
//
// V(3) is avoided in favor of V(2) (if actionable) or V(4) (if diagnostic).
//
// Production deployments use V(2) by default. Set --v=4 for troubleshooting.
package mount
