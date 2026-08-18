#!/bin/zsh
set -euo pipefail

SCRIPT_DIR="${0:A:h}"
ROOT_DIR="${SCRIPT_DIR:h}"
BUILD_DIR="${ROOT_DIR}/build"
STAGE_DIR="${BUILD_DIR}/module"
DIST_DIR="${ROOT_DIR}/dist"
SDK_ROOT="${ANDROID_SDK_ROOT:-${ANDROID_HOME:-$HOME/Library/Android/sdk}}"
VERSION=$(sed -n 's/^version=//p' "${ROOT_DIR}/module/module.prop" | head -n 1)
OUTPUT_NAME="clipboard-assistant-kernelsu-arm64-v${VERSION}.zip"

if [[ -z "${VERSION}" ]]; then
    echo "module.prop does not contain a version" >&2
    exit 1
fi

PLATFORM_DIR=$(find "${SDK_ROOT}/platforms" -maxdepth 1 -type d -name 'android-*' | sort -V | tail -n 1)
BUILD_TOOLS_DIR=$(find "${SDK_ROOT}/build-tools" -maxdepth 1 -type d | sort -V | tail -n 1)

if [[ -z "${PLATFORM_DIR}" || ! -f "${PLATFORM_DIR}/android.jar" ]]; then
    echo "Android platform SDK was not found under ${SDK_ROOT}" >&2
    exit 1
fi
if [[ -z "${BUILD_TOOLS_DIR}" || ! -x "${BUILD_TOOLS_DIR}/d8" ]]; then
    echo "Android d8 was not found under ${SDK_ROOT}" >&2
    exit 1
fi

rm -rf "${BUILD_DIR}" "${DIST_DIR}"
mkdir -p "${BUILD_DIR}/classes" "${BUILD_DIR}/dex" "${STAGE_DIR}" "${DIST_DIR}"

javac \
    -source 8 \
    -target 8 \
    -Xlint:-options \
    -bootclasspath "${PLATFORM_DIR}/android.jar" \
    -d "${BUILD_DIR}/classes" \
    "${ROOT_DIR}/bridge/src/hair/zhy/fastcopy/ClipboardBridge.java" \
    "${ROOT_DIR}/bridge/src/hair/zhy/fastcopy/DnsResolver.java"

CLASS_FILES=("${BUILD_DIR}"/classes/**/*.class(N))
"${BUILD_TOOLS_DIR}/d8" \
    --min-api 29 \
    --output "${BUILD_DIR}/dex" \
    "${CLASS_FILES[@]}"

jar --create \
    --file "${BUILD_DIR}/fastcopy-bridge.jar" \
    -C "${BUILD_DIR}/dex" classes.dex

cd "${ROOT_DIR}/daemon"
CGO_ENABLED=0 GOOS=android GOARCH=arm64 \
    go build -trimpath -ldflags="-s -w" -o "${BUILD_DIR}/fastcopyd" .

cp -R "${ROOT_DIR}/module/." "${STAGE_DIR}/"
cp "${BUILD_DIR}/fastcopyd" "${STAGE_DIR}/bin/fastcopyd"
cp "${BUILD_DIR}/fastcopy-bridge.jar" "${STAGE_DIR}/bin/fastcopy-bridge.jar"
chmod 0755 \
    "${STAGE_DIR}/service.sh" \
    "${STAGE_DIR}/action.sh" \
    "${STAGE_DIR}/uninstall.sh" \
    "${STAGE_DIR}/customize.sh" \
    "${STAGE_DIR}/bin/fastcopyctl" \
    "${STAGE_DIR}/bin/fastcopyd"
chmod 0644 "${STAGE_DIR}/bin/fastcopy-bridge.jar"

cd "${STAGE_DIR}"
COPYFILE_DISABLE=1 zip -q -r "${DIST_DIR}/${OUTPUT_NAME}" . -x '*.DS_Store'

echo "Built ${DIST_DIR}/${OUTPUT_NAME}"
