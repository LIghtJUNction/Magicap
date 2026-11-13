#!/usr/bin/env bash
set -euo pipefail
cd "$KAM_PROJECT_ROOT"
python scripts/licenses.py
mkdir -p src/magicap/bin
version="${KAM_MODULE_VERSION#v}"
ldflags="-s -w -X github.com/LIghtJUNction/Magicap/internal/buildinfo.Version=$version"
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -trimpath -ldflags="$ldflags" -o src/magicap/bin/magicap-arm64 ./cmd/magicap
ndk="${ANDROID_NDK_HOME:-${ANDROID_NDK_ROOT:-/opt/android-ndk}}"
cc="$ndk/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android26-clang"
[[ -x "$cc" ]] || { echo 'Android NDK required for Android x86_64 build (set ANDROID_NDK_HOME)' >&2; exit 1; }
CGO_ENABLED=1 CC="$cc" GOOS=android GOARCH=amd64 go build -trimpath -ldflags="$ldflags" -o src/magicap/bin/magicap-amd64 ./cmd/magicap
