package e2e

import (
	"fmt"
)

// testSSHPrivateKey is a test RSA key for mock RDS authentication
// This is the same key used across all test suites for consistency
const testSSHPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
NhAAAAAwEAAQAAAQEAuUxbIus8fUPSxG419c2P3JAqRnA8DJe77phQZMCtAc1WXWPPv0fn
SZlYjoOqFBs6b3C5hvISxOva2R/wDvAfrMMWtUbyMKmEaYNQuoekSXOGoFsQ3bfR0INCf1
ZSQZT52kDYbUvGUjVj6VSUXkFK2UEZEh1SKrkR2EtldjTwZu8LJtticxhyqoWgRWT+vLU0
pE7SY1xFi31ybJLUr6654NpybzpBvk/kP02QUd8oDMmIFv47evtAHRoI0Ywpr4wTA6M91R
WlvZkXAYG8SdbZe8PR1S1vDXrOamHUF7dLPtUncdPmnpH4HuhknXk9DdzRCH8EEQ2zX5rk
cM9fV7jsHQAAA8ivcOOYr3DjmAAAAAdzc2gtcnNhAAABAQC5TFsi6zx9Q9LEbjX1zY/ckC
pGcDwMl7vumFBkwK0BzVZdY8+/R+dJmViOg6oUGzpvcLmG8hLE69rZH/AO8B+swxa1RvIw
qYRpg1C6h6RJc4agWxDdt9HQg0J/VlJBlPnaQNhtS8ZSNWPpVJReQUrZQRkSHVIquRHYS2
V2NPBm7wsm22JzGHKqhaBFZP68tTSkTtJjXEWLfXJsktSvrrng2nJvOkG+T+Q/TZBR3ygM
yYgW/jt6+0AdGgjRjCmvjBMDoz3VFaW9mRcBgbxJ1tl7w9HVLW8Nes5qYdQXt0s+1Sdx0+
aekfge6GSdeT0N3NEIfwQRDbNfmuRwz19XuOwdAAAAAwEAAQAAAQEAllRFJ/oyk+nfZ6+G
JYoE6csoEQdjIFBFjpeRuXu7oFendpLQa335PXOkLdLRvAgvC1QnoDxqT8qNPVO03VmgSP
fpR15shGAy5as8Zmg/N7v6/8OB1m8YUJL88vPkPgKQBapQK7OrDOz1xsnwqNtNzx4KKfER
xUlqGdFpWlIuq0Joj19iUrCObp0NAD9YqvfB1KyCnCXwaMnhUT9rw7ZDdG6x4sxbwT2gDs
fsuaxb8CZfcOoC4CwHhBvnOHR7eFvqsOv4QZJ9LkuTLV1DGQragGOy3HS3Obbu4ePhbTkN
+ab1GPq+eZMQmDh4+BCDGWgcpcATH0UyVvPXrD0jG5GFvQAAAIATHVBodA7W+pD+aG6ck/
g+hqoXTzRYaD4RdmKU9YQztjvYUQsnfGjYMnMwnN7tYeCXFDYgzPbUc2JUkxAqNmqMof/d
07FoG8Bfu4GAvxDtpGrdYSbOYYXiD6/Oosb0+5ayhcT2uZFWxIy23+Q/Cpcm6qjSSUQGZU
EafLsnKNrvtwAAAIEA3lw8ZXQBmnTI3VJEJuJcM6v7dnzkE/n6ifMhQicaXoUni51Abfki
sA0yaQF1PTIwMOVjGOB7I3DUNIGDLEx79+HnGdSpLY9RNNQe0ZCdvp/akyRY7bTLJqnUZC
xC1e1p1LcjMph+hdDHLlQpS1dH1M4lhkuBdTTLrLx9gQcr6a8AAACBANVUv9OwimPx/Efb
ZPCXDHVgUJbOfmll0fUjBMMJm3tlx8b49xqrrOWO0Nk9GdztTF4nlsYg/563Vw4kU9T7S+
krQ96352S167OB47OvBUozv0hKcJlU9W3TYmt/zXC3asLS4Xhyv7/YxI3wGIs9TNDAKPaT
AUTw5BKMQuNGfVXzAAAADHRlc3RAcmRzLWNzaQECAwQFBg==
-----END OPENSSH PRIVATE KEY-----`

// Test volume sizes for different scenarios
const (
	smallVolumeSize  = 1 * GiB  // 1 GiB for quick tests
	mediumVolumeSize = 5 * GiB  // 5 GiB for block volume tests
	largeVolumeSize  = 10 * GiB // 10 GiB for expansion tests
)

// stagingPath returns a unique staging path for a volume
// This is where NodeStageVolume will mount the volume
func stagingPath(volumeID string) string {
	return fmt.Sprintf("/tmp/csi-e2e-staging/%s", volumeID)
}

// publishPath returns a unique publish path for a volume
// This is where NodePublishVolume will bind-mount the volume
func publishPath(volumeID string) string {
	return fmt.Sprintf("/tmp/csi-e2e-publish/%s", volumeID)
}
