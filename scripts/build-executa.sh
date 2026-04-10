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
BUILD_ALL=false
RUN_TEST=false
PACKAGE=false

for arg in "$@"; do
    case "$arg" in
        --all)     BUILD_ALL=true ;;
        --test)    RUN_TEST=true ;;
        --package) PACKAGE=true; BUILD_ALL=true ;;
        --help|-h)
            echo "Usage: $0 [--all] [--test] [--package]"
            exit 0
            ;;
        *)
            echo "Unknown argument: $arg" >&2
            exit 1
            ;;
    esac
done

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
            -o "dist/${PLUGIN_NAME}-${platform_key}${suffix}" .
    else
        GOOS="${goos}" GOARCH="${goarch}" \
            go build -ldflags "$(ldflags_for "${goos}" "${goarch}")" \
            -o "dist/${PLUGIN_NAME}-${platform_key}${suffix}" .
    fi
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

if [[ "${BUILD_ALL}" == "true" ]]; then
    build_one "darwin-arm64" "darwin" "arm64"
    build_one "darwin-x86_64" "darwin" "amd64"
    build_one "linux-x86_64" "linux" "amd64"
    build_one "linux-aarch64" "linux" "arm64"
    build_one "linux-armv7l" "linux" "arm" "7"
    build_one "windows-x86_64" "windows" "amd64"
    build_one "windows-arm64" "windows" "arm64"
    echo ""
    echo -e "${GREEN}全平台构建完成！${NC}"
    ls -lh dist/
else
    host_goos="$(go env GOOS)"
    host_goarch="$(go env GOARCH)"
    go build -ldflags "$(ldflags_for "${host_goos}" "${host_goarch}")" -o "dist/${PLUGIN_NAME}" .
    size="$(du -h "dist/${PLUGIN_NAME}" | cut -f1)"
    echo -e "${GREEN}构建成功！${NC} dist/${PLUGIN_NAME} (${size})"
fi

if [[ "${PACKAGE}" == "true" ]]; then
    echo ""
    echo -e "${GREEN}打包...${NC}"
    mkdir -p dist/packages
    for f in dist/${PLUGIN_NAME}-*; do
        base="$(basename "${f}")"
        plat="${base#${PLUGIN_NAME}-}"
        plat="${plat%.exe}"
        if [[ "${f}" == *.exe ]]; then
            (cd dist && zip -j "packages/${PLUGIN_NAME}-${plat}.zip" "${base}")
        else
            (cd dist && tar czf "packages/${PLUGIN_NAME}-${plat}.tar.gz" "${base}")
        fi
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
        result="$(echo '{"jsonrpc":"2.0","method":"describe","id":1}' | "${binary}" executa 2>/dev/null)"
        if echo "${result}" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['result']['name']=='yutu-executa'; assert d['result']['tools'][0]['name']=='run_yutu'" 2>/dev/null; then
            echo -e "  ${GREEN}✅ describe 通过${NC}"
        else
            echo -e "  ${RED}❌ describe 失败${NC}"
            exit 1
        fi

        echo -e "  [health]..."
        result="$(echo '{"jsonrpc":"2.0","method":"health","id":2}' | "${binary}" executa 2>/dev/null)"
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
        result="$(printf '%s\n' "${request}" | "${binary}" executa 2>/dev/null)"
        if echo "${result}" | python3 -c "import sys,json,os; d=json.load(sys.stdin); assert d['result']['success'] is True; path=d['result']['data']['output_file']; assert os.path.isfile(path)" 2>/dev/null; then
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
echo -e "  Local run: ./dist/${PLUGIN_NAME} executa"
echo ""
