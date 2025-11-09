#!/usr/bin/env bash
#
# deploy-dev.sh - Deploy RDS CSI Driver with :dev tag
#
# This script redeploys the RDS CSI driver with the :dev container image tag.
# Used for testing security fixes and development builds.
#
# Usage:
#   ./scripts/deploy-dev.sh [--wait]
#
# Options:
#   --wait    Wait for rollout to complete before exiting
#

set -euo pipefail

# Configuration
NAMESPACE="kube-system"
REGISTRY="ghcr.io/3whiskeywhiskey"
IMAGE_NAME="rds-csi-driver"
IMAGE_TAG="dev"
FULL_IMAGE="${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Parse arguments
WAIT_FOR_ROLLOUT=false
if [[ "${1:-}" == "--wait" ]]; then
    WAIT_FOR_ROLLOUT=true
fi

echo -e "${GREEN}=== RDS CSI Driver - Deploy Dev Image ===${NC}"
echo ""
echo "Registry:   ${REGISTRY}"
echo "Image:      ${IMAGE_NAME}"
echo "Tag:        ${IMAGE_TAG}"
echo "Namespace:  ${NAMESPACE}"
echo ""

# Check if kubectl is available
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}Error: kubectl not found${NC}"
    exit 1
fi

# Check if cluster is accessible
if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}Error: Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi

# Update controller deployment
echo -e "${YELLOW}Updating controller deployment...${NC}"
kubectl set image deployment/rds-csi-controller \
    rds-csi-driver="${FULL_IMAGE}" \
    -n "${NAMESPACE}"

# Update node daemonset
echo -e "${YELLOW}Updating node daemonset...${NC}"
kubectl set image daemonset/rds-csi-node \
    rds-csi-driver="${FULL_IMAGE}" \
    -n "${NAMESPACE}"

echo ""
echo -e "${GREEN}Image updates applied${NC}"

# Wait for rollout if requested
if [[ "${WAIT_FOR_ROLLOUT}" == "true" ]]; then
    echo ""
    echo -e "${YELLOW}Waiting for controller rollout...${NC}"
    kubectl rollout status deployment/rds-csi-controller -n "${NAMESPACE}" --timeout=5m

    echo -e "${YELLOW}Waiting for node rollout...${NC}"
    kubectl rollout status daemonset/rds-csi-node -n "${NAMESPACE}" --timeout=5m

    echo ""
    echo -e "${GREEN}Rollout complete!${NC}"
else
    echo ""
    echo "To monitor rollout status, run:"
    echo "  kubectl rollout status deployment/rds-csi-controller -n ${NAMESPACE}"
    echo "  kubectl rollout status daemonset/rds-csi-node -n ${NAMESPACE}"
fi

# Show current pod status
echo ""
echo -e "${YELLOW}Current pod status:${NC}"
kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name=rds-csi-driver

echo ""
echo -e "${GREEN}Deployment update initiated${NC}"
echo ""
echo "Next steps:"
echo "  1. Monitor pod status: kubectl get pods -n ${NAMESPACE} -w"
echo "  2. Check logs: kubectl logs -f deployment/rds-csi-controller -n ${NAMESPACE}"
echo "  3. Run tests: ./scripts/test-security-fix.sh"
