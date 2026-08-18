#!/bin/zsh
set -euo pipefail

SCRIPT_DIR="${0:A:h}"
ROOT_DIR="${SCRIPT_DIR:h}"
DIST_DIR="${ROOT_DIR}/dist"
APP_DIR="${DIST_DIR}/FastCopy.app"
VERSION=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "${ROOT_DIR}/Info.plist")
ZIP_PATH="${DIST_DIR}/FastCopy-macos-v${VERSION}.zip"

cd "${ROOT_DIR}"
swift build -c release
BIN_DIR="$(swift build -c release --show-bin-path)"

rm -rf "${APP_DIR}"
mkdir -p "${APP_DIR}/Contents/MacOS"
cp "${ROOT_DIR}/Info.plist" "${APP_DIR}/Contents/Info.plist"
cp "${BIN_DIR}/FastCopyMac" "${APP_DIR}/Contents/MacOS/FastCopyMac"
codesign --force --deep --sign - "${APP_DIR}"
rm -f "${ZIP_PATH}"
ditto -c -k --sequesterRsrc --keepParent "${APP_DIR}" "${ZIP_PATH}"

echo "Built ${APP_DIR}"
echo "Built ${ZIP_PATH}"
