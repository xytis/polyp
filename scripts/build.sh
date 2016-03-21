#!/usr/bin/env bash
#
# This script builds the application from source for multiple platforms.
set -e

# Get the parent directory of where this script is.
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ] ; do SOURCE="$(readlink "$SOURCE")"; done
DIR="$( cd -P "$( dirname "$SOURCE" )/.." && pwd )"

# Change into that directory
cd "$DIR"

# Get the git commit
GIT_COMMIT=$(git rev-parse HEAD)
GIT_DIRTY=$(test -n "`git status --porcelain`" && echo "+CHANGES" || true)
GIT_DESCRIBE=$(git describe --tags)

# Determine the arch/os combos we're building for
#XC_ARCH=${XC_ARCH:-"386 amd64"}
#XC_OS=${XC_OS:-"solaris darwin freebsd linux"}
XC_ARCH=${XC_ARCH:-"amd64"}
XC_OS=${XC_OS:-"linux"}

# Install dependencies
echo "==> Getting dependencies..."
go get ./...

# Delete the old dir
echo "==> Removing old directory..."
rm -f bin/*
rm -rf pkg/*
mkdir -p bin/

# If it's dev mode, only build for ourself
if [ "${POLYP_DEV}x" != "x" ]; then
  XC_OS=$(go env GOOS)
  XC_ARCH=$(go env GOARCH)
fi

# Build!
echo "==> Building..."
$GOPATH/bin/gox \
  -os="${XC_OS}" \
  -arch="${XC_ARCH}" \
  -ldflags "-X main.GitCommit ${GIT_COMMIT}${GIT_DIRTY} -X main.GitDescribe ${GIT_DESCRIBE}" \
  -output "pkg/{{.OS}}_{{.Arch}}/polyp" \
  .

# Move all the compiled things to the $GOPATH/bin
GOPATH=${GOPATH:-$(go env GOPATH)}
case $(uname) in
  CYGWIN*)
    GOPATH="$(cygpath $GOPATH)"
    ;;
esac
OLDIFS=$IFS
IFS=: MAIN_GOPATH=($GOPATH)
IFS=$OLDIFS

# Copy our OS/Arch to the bin/ directory
DEV_PLATFORM="./pkg/$(go env GOOS)_$(go env GOARCH)"
for F in $(find ${DEV_PLATFORM} -mindepth 1 -maxdepth 1 -type f); do
  cp ${F} bin/
  cp ${F} ${MAIN_GOPATH}/bin/
done

if [ "${POLYP_DEV}x" = "x" ]; then
  # Zip and copy to the dist dir
  echo "==> Packaging..."
  for PLATFORM in $(find ./pkg -mindepth 1 -maxdepth 1 -type d); do
    OSARCH=$(basename ${PLATFORM})
    echo "--> ${OSARCH}"

    pushd $PLATFORM >/dev/null 2>&1
    zip ../${OSARCH}.zip ./*
    popd >/dev/null 2>&1
  done
fi

# Done!
echo
echo "==> Results:"
ls -hl bin/