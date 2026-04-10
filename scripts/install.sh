#!/usr/bin/env bash
set -euo pipefail

# OpenClaw 中国区一键安装脚本
# 用法: curl -fsSL https://raw.githubusercontent.com/goodtiger/openclaw-install/main/scripts/install.sh | bash
# 或:   wget -qO- https://raw.githubusercontent.com/goodtiger/openclaw-install/main/scripts/install.sh | bash

REPO="goodtiger/openclaw-install"
BINARY_NAME="openclaw-install"
INSTALL_DIR="/usr/local/bin"

# 下载源优先级：ghproxy → GitHub
DOWNLOAD_SOURCES=(
  "ghproxy|https://ghproxy.com/https://github.com/${REPO}/releases/latest/download"
  "GitHub|https://github.com/${REPO}/releases/latest/download"
)

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()    { echo -e "${BLUE}==>${NC} $*"; }
success() { echo -e "${GREEN}==>${NC} $*"; }
warn()    { echo -e "${YELLOW}==>${NC} $*"; }
error()   { echo -e "${RED}==>${NC} $*" >&2; }

detect_os_arch() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    darwin|linux) ;;
    *)
      error "不支持的操作系统: $os"
      exit 1
      ;;
  esac

  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      error "不支持的架构: $arch"
      exit 1
      ;;
  esac

  OS="$os"
  ARCH="$arch"
}

detect_latest_version() {
  local urls=(
    "https://api.github.com/repos/${REPO}/releases/latest"
    "https://ghproxy.com/https://api.github.com/repos/${REPO}/releases/latest"
  )

  for url in "${urls[@]}"; do
    local response
    response="$(curl -fsSL --connect-timeout 5 --max-time 15 "$url" 2>/dev/null)" || continue
    local tag
    tag="$(echo "$response" | grep -o '"tag_name":"[^"]*"' | head -1 | cut -d'"' -f4)" || continue
    if [[ -n "$tag" ]]; then
      VERSION="${tag#v}"
      return 0
    fi
  done

  error "无法获取最新版本信息，请手动指定版本: $0 --version <version>"
  exit 1
}

download_with_fallback() {
  local filename="$1"
  local output="$2"
  local version="$3"

  local sources=(
    "ghproxy|https://ghproxy.com/https://github.com/${REPO}/releases/download/v${version}/${filename}"
    "GitHub|https://github.com/${REPO}/releases/download/v${version}/${filename}"
  )

  for source_entry in "${sources[@]}"; do
    local name="${source_entry%%|*}"
    local url="${source_entry#*|}"

    info "尝试从 $name 下载..."
    if curl -fsSL --connect-timeout 10 --max-time 120 --progress-bar "$url" -o "$output" 2>/dev/null; then
      success "从 $name 下载成功"
      return 0
    fi
    warn "从 $name 下载失败，继续尝试下一个源..."
  done

  return 1
}

install_binary() {
  local version="$1"
  local ext=""
  [[ "$OS" == "windows" ]] && ext=".exe"

  local archive_name="${BINARY_NAME}_${version}_${OS}_${ARCH}"
  local archive_file=""

  if [[ "$OS" == "windows" ]]; then
    archive_file="${archive_name}.zip"
  else
    archive_file="${archive_name}.tar.gz"
  fi

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' EXIT

  if ! download_with_fallback "$archive_file" "$tmp_dir/$archive_file" "$version"; then
    error "所有下载源均失败，请检查网络连接或手动下载: https://github.com/${REPO}/releases"
    exit 1
  fi

  local binary_path="$tmp_dir/${BINARY_NAME}${ext}"

  if [[ "$OS" == "windows" ]]; then
    unzip -q "$tmp_dir/$archive_file" -d "$tmp_dir"
    binary_path="$(find "$tmp_dir" -name "${BINARY_NAME}.exe" -type f | head -1)"
  else
    tar -xzf "$tmp_dir/$archive_file" -C "$tmp_dir"
    binary_path="$tmp_dir/${archive_name}/${BINARY_NAME}"
  fi

  if [[ ! -f "$binary_path" ]]; then
    error "解压后未找到二进制文件"
    exit 1
  fi

  chmod +x "$binary_path"

  local install_path="${INSTALL_DIR}/${BINARY_NAME}"
  if [[ -w "$INSTALL_DIR" ]] || command -v sudo >/dev/null 2>&1; then
    if [[ -w "$INSTALL_DIR" ]]; then
      cp "$binary_path" "$install_path"
    else
      sudo cp "$binary_path" "$install_path"
    fi
    success "安装完成: $install_path"
  else
    cp "$binary_path" "./${BINARY_NAME}"
    warn "无权限写入 $INSTALL_DIR，已安装到当前目录: ./${BINARY_NAME}"
    warn "请手动移动: sudo mv ./${BINARY_NAME} ${INSTALL_DIR}/"
  fi
}

usage() {
  cat <<EOF
用法: $0 [选项]

选项:
  -v, --version VERSION  指定安装版本（默认: 最新版本）
  -h, --help             显示此帮助信息

示例:
  $0                     # 安装最新版本
  $0 --version 0.2.0     # 安装指定版本
EOF
}

main() {
  local version=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      -v|--version)
        version="$2"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        error "未知选项: $1"
        usage
        exit 1
        ;;
    esac
  done

  info "OpenClaw 中国区安装器"

  detect_os_arch
  info "检测到系统: ${OS}/${ARCH}"

  if [[ -z "$version" ]]; then
    detect_latest_version
    info "最新版本: v${version}"
  else
    info "指定版本: v${version}"
  fi

  install_binary "$version"

  echo ""
  success "安装完成！"
  echo ""
  echo "快速开始:"
  echo "  ${BINARY_NAME} doctor          # 环境诊断"
  echo "  ${BINARY_NAME} install         # 交互式安装 OpenClaw"
  echo "  ${BINARY_NAME} install --yes   # 快速安装"
  echo ""
  echo "文档: https://github.com/${REPO}"
}

main "$@"
