#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"
PACKAGES_DIR="$DIST_DIR/packages"
ARCHIVES_DIR="$DIST_DIR/archives"
APP_VERSION_FILE="$ROOT_DIR/internal/app/version.go"
GOCACHE_DIR="${GOCACHE:-$DIST_DIR/.gocache}"

DEFAULT_TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

usage() {
  cat <<'EOF'
Usage:
  scripts/build-release.sh [target...]

Targets use the form <goos>/<goarch>, for example:
  scripts/build-release.sh
  scripts/build-release.sh linux/amd64
  scripts/build-release.sh linux/amd64 darwin/arm64 windows/amd64

If no targets are provided, the default release set is built:
  linux/amd64
  linux/arm64
  darwin/amd64
  darwin/arm64
  windows/amd64
EOF
}

require_cmd() {
  local name=$1
  command -v "$name" >/dev/null 2>&1 || {
    echo "missing required command: $name" >&2
    exit 1
  }
}

read_version() {
  local version
  version=$(awk -F'"' '/const Version =/ { print $2; exit }' "$APP_VERSION_FILE")
  if [[ -z "$version" ]]; then
    echo "failed to read version from $APP_VERSION_FILE" >&2
    exit 1
  fi
  printf '%s\n' "$version"
}

checksum_file() {
  local file=$1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$(basename "$file")"
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$(basename "$file")"
    return
  fi
  echo "missing checksum command: sha256sum or shasum" >&2
  exit 1
}

archive_target() {
  local package_name=$1
  local goos=$2
  local archive_path

  if [[ "$goos" == "windows" ]]; then
    archive_path="$ARCHIVES_DIR/$package_name.zip"
    (
      cd "$PACKAGES_DIR"
      zip -rq "$archive_path" "$package_name"
    )
  else
    archive_path="$ARCHIVES_DIR/$package_name.tar.gz"
    tar -C "$PACKAGES_DIR" -czf "$archive_path" "$package_name"
  fi

  printf '%s\n' "$archive_path"
}

build_target() {
  local version=$1
  local target=$2
  local goos=${target%/*}
  local goarch=${target#*/}
  local binary_name="openclaw-install"
  local package_name="openclaw-install_${version}_${goos}_${goarch}"
  local package_dir="$PACKAGES_DIR/$package_name"

  if [[ "$goos" == "windows" ]]; then
    binary_name+=".exe"
  fi

  rm -rf "$package_dir"
  mkdir -p "$package_dir"

  echo "==> Building $target" >&2
  env GOCACHE="$GOCACHE_DIR" CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w" \
    -o "$package_dir/$binary_name" \
    ./cmd/openclaw-install

  cp "$ROOT_DIR/README.md" "$package_dir/README.md"
  cp "$ROOT_DIR/TESTING.md" "$package_dir/TESTING.md"

  archive_target "$package_name" "$goos"
}

main() {
  if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
  fi

  require_cmd go
  require_cmd tar
  require_cmd zip

  local version
  version=$(read_version)

  local targets=()
  if [[ "$#" -eq 0 ]]; then
    targets=("${DEFAULT_TARGETS[@]}")
  else
    targets=("$@")
  fi

  mkdir -p "$PACKAGES_DIR" "$ARCHIVES_DIR"
  mkdir -p "$GOCACHE_DIR"

  local archives=()
  local target
  for target in "${targets[@]}"; do
    if [[ "$target" != */* ]]; then
      echo "invalid target: $target" >&2
      echo "expected format: <goos>/<goarch>" >&2
      exit 1
    fi
    archives+=("$(build_target "$version" "$target")")
  done

  local sums_file="$ARCHIVES_DIR/SHA256SUMS"
  : > "$sums_file"
  (
    cd "$ARCHIVES_DIR"
    local archive
    for archive in "${archives[@]}"; do
      checksum_file "$archive" >> "$sums_file"
    done
  )

  echo ""
  echo "Release artifacts:"
  printf '  %s\n' "${archives[@]}"
  echo "  $sums_file"
}

main "$@"
