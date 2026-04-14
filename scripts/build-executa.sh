#!/usr/bin/env bash
# Copyright 2026 eat-pray-ai & OpenWaygate
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

PLUGIN_NAME="yutu-executa"
CMD_PKG="github.com/eat-pray-ai/yutu/cmd"
PLUGIN_PKG="./cmd/yutu-executa"
BUILD_ALL=false
RUN_TEST=false
PACKAGE=false
declare -a SELECTED_PLATFORMS=()

ALL_PLATFORMS=(
    "darwin-arm64"
    "darwin-amd64"
    "linux-amd64"
    "linux-arm64"
    "linux-armv7"
    "windows-amd64"
    "windows-arm64"
)

usage() {
    cat <<'EOF'
Usage: ./scripts/build-executa.sh [--all] [--platform <name>] [--test] [--package]

Options:
  --all                 Build all supported targets
  --platform <name>     Build only the selected target; may be repeated
  --test                Run lightweight protocol tests after building
  --package             Create archives and .sha256 files for built binaries
  --help, -h            Show this help

Supported platform names:
  darwin-arm64
  darwin-amd64
  linux-amd64
  linux-arm64
  linux-armv7
  windows-amd64
  windows-arm64
EOF
}

parse_platform() {
    local platform_key="$1"

    case "${platform_key}" in
        darwin-arm64)
            printf '%s %s %s %s\n' "darwin-arm64" "darwin" "arm64" ""
            ;;
        darwin-amd64)
            printf '%s %s %s %s\n' "darwin-amd64" "darwin" "amd64" ""
            ;;
        linux-amd64)
            printf '%s %s %s %s\n' "linux-amd64" "linux" "amd64" ""
            ;;
        linux-arm64)
            printf '%s %s %s %s\n' "linux-arm64" "linux" "arm64" ""
            ;;
        linux-armv7)
            printf '%s %s %s %s\n' "linux-armv7" "linux" "arm" "7"
            ;;
        windows-amd64)
            printf '%s %s %s %s\n' "windows-amd64" "windows" "amd64" ""
            ;;
        windows-arm64)
            printf '%s %s %s %s\n' "windows-arm64" "windows" "arm64" ""
            ;;
        *)
            echo "Unsupported platform: ${platform_key}" >&2
            exit 1
            ;;
    esac
}

while [[ "$#" -gt 0 ]]; do
    case "$1" in
        --all)
            BUILD_ALL=true
            shift
            ;;
        --platform)
            if [[ "$#" -lt 2 ]]; then
                echo "--platform requires a value" >&2
                exit 1
            fi
            parse_platform "$2" >/dev/null
            SELECTED_PLATFORMS+=("$2")
            shift 2
            ;;
        --test)
            RUN_TEST=true
            shift
            ;;
        --package)
            PACKAGE=true
            shift
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

if [[ "${BUILD_ALL}" == "true" && "${#SELECTED_PLATFORMS[@]}" -gt 0 ]]; then
    echo "--all cannot be combined with --platform" >&2
    exit 1
fi

if [[ "${PACKAGE}" == "true" && "${BUILD_ALL}" != "true" && "${#SELECTED_PLATFORMS[@]}" -eq 0 ]]; then
    BUILD_ALL=true
fi

version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
commit="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
commit_date="$(git log -1 --date='format:%Y-%m-%dT%H:%M:%SZ' --pretty=%cd 2>/dev/null || date -u +%Y-%m-%dT%H:%M:%SZ)"
builder="${GITHUB_ACTOR:-${USER:-unknown}}"

ldflags_for() {
    local goos="$1"
    local goarch="$2"
    printf '%s' "-s -w -X ${CMD_PKG}.Version=${version} -X ${CMD_PKG}.Commit=${commit} -X ${CMD_PKG}.CommitDate=${commit_date} -X ${CMD_PKG}.Os=${goos} -X ${CMD_PKG}.Arch=${goarch} -X ${CMD_PKG}.Builder=${builder}"
}

build_one() {
    local platform_key="$1"
    local goos="$2"
    local goarch="$3"
    local goarm="${4:-}"
    local suffix=""

    if [[ "${goos}" == "windows" ]]; then
        suffix=".exe"
    fi

    echo -e "  Building ${platform_key}..."
    if [[ -n "${goarm}" ]]; then
        GOOS="${goos}" GOARCH="${goarch}" GOARM="${goarm}" \
            go build -ldflags "$(ldflags_for "${goos}" "${goarch}")" \
            -o "dist/${PLUGIN_NAME}-${platform_key}${suffix}" "${PLUGIN_PKG}"
    else
        GOOS="${goos}" GOARCH="${goarch}" \
            go build -ldflags "$(ldflags_for "${goos}" "${goarch}")" \
            -o "dist/${PLUGIN_NAME}-${platform_key}${suffix}" "${PLUGIN_PKG}"
    fi
}

checksum_file() {
    local file_path="$1"
    local checksum_path="${file_path}.sha256"
    local file_name
    local hash=""
    file_name="$(basename "${file_path}")"

    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "${file_path}" | awk -v name="${file_name}" '{print $1 "  " name}' > "${checksum_path}"
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "${file_path}" | awk -v name="${file_name}" '{print $1 "  " name}' > "${checksum_path}"
    elif command -v pwsh >/dev/null 2>&1; then
        hash="$(pwsh -NoLogo -NoProfile -Command "(Get-FileHash -Algorithm SHA256 -LiteralPath '${file_path}').Hash.ToLower()")"
        printf '%s  %s\n' "${hash}" "${file_name}" > "${checksum_path}"
    elif command -v powershell.exe >/dev/null 2>&1; then
        hash="$(powershell.exe -NoLogo -NoProfile -Command "(Get-FileHash -Algorithm SHA256 -LiteralPath '${file_path}').Hash.ToLower()" | tr -d '\r')"
        printf '%s  %s\n' "${hash}" "${file_name}" > "${checksum_path}"
    else
        echo "No SHA256 tool available for ${file_path}" >&2
        exit 1
    fi
}

package_binary() {
    local binary_path="$1"
    local platform_key="$2"
    local package_path=""

    mkdir -p dist/packages
    if [[ "${binary_path}" == *.exe ]]; then
        package_path="dist/packages/${PLUGIN_NAME}-${platform_key}.zip"
        if command -v zip >/dev/null 2>&1; then
            (cd dist && zip -jq "packages/${PLUGIN_NAME}-${platform_key}.zip" "$(basename "${binary_path}")")
        elif command -v pwsh >/dev/null 2>&1; then
            pwsh -NoLogo -NoProfile -Command "Compress-Archive -Force -LiteralPath '${binary_path}' -DestinationPath '${package_path}'"
        elif command -v powershell.exe >/dev/null 2>&1; then
            powershell.exe -NoLogo -NoProfile -Command "Compress-Archive -Force -LiteralPath '${binary_path}' -DestinationPath '${package_path}'" >/dev/null
        else
            echo "No ZIP tool available for ${binary_path}" >&2
            exit 1
        fi
    else
        package_path="dist/packages/${PLUGIN_NAME}-${platform_key}.tar.gz"
        (cd dist && tar czf "packages/${PLUGIN_NAME}-${platform_key}.tar.gz" "$(basename "${binary_path}")")
    fi

    checksum_file "${package_path}"
}

echo -e "${CYAN}============================================================${NC}"
echo -e "${CYAN}  Yutu Executa Binary Builder${NC}"
echo -e "${CYAN}============================================================${NC}"
echo -e "  Plugin:   ${PLUGIN_NAME}"
echo -e "  Version:  ${version}"
echo -e "  Platform: $(uname -s) $(uname -m)"
echo -e "  Go:       $(go version 2>/dev/null || echo 'not installed')"
echo ""

rm -rf dist/
mkdir -p dist

declare -a BUILT_BINARIES=()

if [[ "${BUILD_ALL}" == "true" || "${#SELECTED_PLATFORMS[@]}" -gt 0 ]]; then
    if [[ "${BUILD_ALL}" == "true" ]]; then
        SELECTED_PLATFORMS=("${ALL_PLATFORMS[@]}")
    fi

    for platform_key in "${SELECTED_PLATFORMS[@]}"; do
        read -r normalized_key goos goarch goarm <<<"$(parse_platform "${platform_key}")"
        build_one "${normalized_key}" "${goos}" "${goarch}" "${goarm}"
        if [[ "${goos}" == "windows" ]]; then
            BUILT_BINARIES+=("dist/${PLUGIN_NAME}-${normalized_key}.exe")
        else
            BUILT_BINARIES+=("dist/${PLUGIN_NAME}-${normalized_key}")
        fi
    done

    echo ""
    if [[ "${#BUILT_BINARIES[@]}" -eq 1 ]]; then
        echo -e "${GREEN}目标平台构建完成！${NC}"
    else
        echo -e "${GREEN}全平台构建完成！${NC}"
    fi
    ls -lh dist/
else
    host_goos="$(go env GOOS)"
    host_goarch="$(go env GOARCH)"
    host_binary="dist/${PLUGIN_NAME}"
    if [[ "${host_goos}" == "windows" ]]; then
        host_binary="${host_binary}.exe"
    fi
    go build -ldflags "$(ldflags_for "${host_goos}" "${host_goarch}")" -o "${host_binary}" "${PLUGIN_PKG}"
    BUILT_BINARIES+=("${host_binary}")
    size="$(du -h "${host_binary}" | cut -f1)"
    echo -e "${GREEN}构建成功！${NC} ${host_binary} (${size})"
fi

if [[ "${PACKAGE}" == "true" ]]; then
    echo ""
    echo -e "${GREEN}打包...${NC}"
    for binary_path in "${BUILT_BINARIES[@]}"; do
        base="$(basename "${binary_path}")"
        platform_key="${base#${PLUGIN_NAME}-}"
        platform_key="${platform_key%.exe}"
        package_binary "${binary_path}" "${platform_key}"
    done
    echo ""
    ls -lh dist/packages/
fi

if [[ "${RUN_TEST}" == "true" ]]; then
    binary="dist/${PLUGIN_NAME}"
    if [[ ! -f "${binary}" ]]; then
        binary="$(find dist -maxdepth 1 -type f -name "${PLUGIN_NAME}-*" ! -name '*.exe' | head -1)"
    fi

    if [[ -f "${binary}" && -x "${binary}" ]]; then
        echo ""
        echo -e "${CYAN}── 协议测试 ──────────────────────────────────${NC}"

        echo -e "  [describe]..."
        result="$(echo '{"jsonrpc":"2.0","method":"describe","id":1}' | "${binary}" 2>/dev/null)"
        if echo "${result}" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['name']=='yutu-executa'; assert d['result']['tools'][0]['name']=='run_yutu'" 2>/dev/null; then
            echo -e "  ${GREEN}✅ describe 通过${NC}"
        else
            echo -e "  ${RED}❌ describe 失败${NC}"
            exit 1
        fi

        echo -e "  [health]..."
        result="$(echo '{"jsonrpc":"2.0","method":"health","id":2}' | "${binary}" 2>/dev/null)"
        if echo "${result}" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['status']=='healthy'" 2>/dev/null; then
            echo -e "  ${GREEN}✅ health 通过${NC}"
        else
            echo -e "  ${RED}❌ health 失败${NC}"
            exit 1
        fi

        echo -e "  [invoke]..."
        tmpdir="$(mktemp -d)"
        trap 'rm -rf "${tmpdir}"' EXIT
        request="$(python3 - "${tmpdir}" <<'PY'
import json, sys
print(json.dumps({
    "jsonrpc": "2.0",
    "method": "invoke",
    "id": 3,
    "params": {
        "tool": "run_yutu",
        "arguments": {
            "command": ["version"],
            "cwd": sys.argv[1],
        }
    }
}))
PY
)"
        result="$(printf '%s\n' "${request}" | "${binary}" 2>/dev/null)"
        if echo "${result}" | python3 -c "import sys,json,os; d=json.load(sys.stdin); path=d['__file_transport']; assert os.path.isfile(path); resp=json.load(open(path)); assert resp['result']['success'] is True" 2>/dev/null; then
            echo -e "  ${GREEN}✅ invoke 通过${NC}"
        else
            echo -e "  ${RED}❌ invoke 失败${NC}"
            exit 1
        fi
    else
        echo -e "${YELLOW}未找到可执行二进制${NC}"
        exit 1
    fi
fi

echo ""
echo -e "${CYAN}── 下一步 ────────────────────────────────────${NC}"
echo -e "  Anna Binary package: dist/packages/${PLUGIN_NAME}-<platform>.tar.gz"
if [[ "${#BUILT_BINARIES[@]}" -eq 1 ]]; then
    echo -e "  Local run: ./${BUILT_BINARIES[0]}"
else
    echo -e "  Local run: ./dist/${PLUGIN_NAME}"
fi
echo ""
