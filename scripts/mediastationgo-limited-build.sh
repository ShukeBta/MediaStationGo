#!/bin/sh
set -eu

BUILDER_NAME="mediastationgo-limited"
BUILDER_CONTAINER="buildx_buildkit_${BUILDER_NAME}0"
MEMORY_BYTES="2147483648"
MEMORY_SWAP_BYTES="2147483648"
CPU_QUOTA="200000"
CPU_PERIOD="100000"
MIN_FREE_MB="${MIN_FREE_MB:-4096}"
DRIVER_OPTIONS="memory=2g,memory-swap=2g,cpu-quota=${CPU_QUOTA},cpu-period=${CPU_PERIOD}"

usage() {
  echo "Usage: $0 IMAGE_TAG [VERSION] [CONTEXT]" >&2
  echo "Example: $0 mediastation-go:local-test local-test ." >&2
  exit 2
}

[ "$#" -ge 1 ] && [ "$#" -le 3 ] || usage

IMAGE_TAG="$1"
VERSION="${2:-dev}"
CONTEXT="${3:-.}"

command -v docker >/dev/null 2>&1 || {
  echo "docker is required" >&2
  exit 3
}

docker buildx version >/dev/null

DOCKER_ROOT="$(docker info --format '{{.DockerRootDir}}')"
AVAILABLE_MB="$(df -Pm "$DOCKER_ROOT" | awk 'NR == 2 { print $4 }')"

case "$AVAILABLE_MB" in
  ''|*[!0-9]*)
    echo "cannot determine free space for Docker root: $DOCKER_ROOT" >&2
    exit 4
    ;;
esac

if [ "$AVAILABLE_MB" -lt "$MIN_FREE_MB" ]; then
  echo "insufficient Docker disk space: available=${AVAILABLE_MB}MB required=${MIN_FREE_MB}MB root=$DOCKER_ROOT" >&2
  exit 5
fi

echo "docker_root=$DOCKER_ROOT available=${AVAILABLE_MB}MB required=${MIN_FREE_MB}MB"

if ! docker buildx inspect "$BUILDER_NAME" >/dev/null 2>&1; then
  docker buildx create \
    --name "$BUILDER_NAME" \
    --driver docker-container \
    --driver-opt "$DRIVER_OPTIONS" >/dev/null
fi

docker buildx inspect "$BUILDER_NAME" --bootstrap >/dev/null

if ! docker container inspect "$BUILDER_CONTAINER" >/dev/null 2>&1; then
  echo "builder container not found: $BUILDER_CONTAINER" >&2
  exit 6
fi

actual_limits="$(docker container inspect "$BUILDER_CONTAINER" \
  --format '{{.HostConfig.Memory}} {{.HostConfig.MemorySwap}} {{.HostConfig.CpuQuota}} {{.HostConfig.CpuPeriod}}')"
expected_limits="$MEMORY_BYTES $MEMORY_SWAP_BYTES $CPU_QUOTA $CPU_PERIOD"

if [ "$actual_limits" != "$expected_limits" ]; then
	echo "updating fixed builder resource limits in place"
	docker update \
		--memory 2g \
		--memory-swap 2g \
		--cpu-period "$CPU_PERIOD" \
		--cpu-quota "$CPU_QUOTA" \
		"$BUILDER_CONTAINER" >/dev/null
	actual_limits="$(docker container inspect "$BUILDER_CONTAINER" \
		--format '{{.HostConfig.Memory}} {{.HostConfig.MemorySwap}} {{.HostConfig.CpuQuota}} {{.HostConfig.CpuPeriod}}')"
	if [ "$actual_limits" != "$expected_limits" ]; then
		echo "builder resource limits do not match after update" >&2
		echo "expected: $expected_limits" >&2
		echo "actual:   $actual_limits" >&2
		exit 7
	fi
fi

echo "builder=$BUILDER_NAME limits=2cpu/2g image=$IMAGE_TAG version=$VERSION"
docker buildx build \
  --builder "$BUILDER_NAME" \
  --load \
  --progress plain \
  --build-arg "VERSION=$VERSION" \
  --tag "$IMAGE_TAG" \
  "$CONTEXT"

df -h "$DOCKER_ROOT"
docker buildx du --builder "$BUILDER_NAME"
