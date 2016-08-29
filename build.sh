#!/bin/sh

GOLANG_BUILDER_IMAGE=golang:1.7
DEFAULT_LOGSPOUT_VERSION=v3.1
BASE_IMAGE=scratch
IMAGE=rtoma/logspout-redis-logstash

usage() {
  echo "Usage: $0 [-d] [-n] [-b <base image>] [-g <golang builder image>] <app version> [<logspout version>]"
  echo
  echo "Parameters:"
  echo "   -d         : Enable development mode. Local sourcefile will be used instead of Github."
  echo "   -n         : Skip building of Docker image, to allow manual building."
  echo "   -b <image> : Set different Docker base image."
  echo "   -g <image> : Set different Golang builder image."
  exit "${1:-0}"
}


DEVMODE=0
BUILDMODE=1
while getopts "dnhg:b:" opt; do
  case $opt in
    d ) DEVMODE=1;;
    n ) BUILDMODE=0;;
    g ) GOLANG_BUILDER_IMAGE="$OPTARG";;
    b ) BASE_IMAGE="$OPTARG";;
    h ) usage;;
    \?) usage 1;;
    : ) usage 1;;
  esac
done
shift "$((OPTIND - 1))"

app_version=$1
logspout_version=${2:-$DEFAULT_LOGSPOUT_VERSION}
if [ -z "$app_version" ]; then
  echo "Missing <app version>"
  usage 1
fi

if [ "$DEVMODE" -eq 1 ]; then
  echo "[*] Running in DEV mode - using local sourcefile"
  app_version="${app_version}-dev"
fi

set -e

# setup clean target dir
targetdir=$PWD/target
[ ! -d "$targetdir" ] && mkdir -p "$targetdir"
cachedir=$PWD/.cache
[ ! -d "$cachedir" ] && mkdir -p "$cachedir"

# remove old artifact
artifact=linux.bin
[ -e "$targetdir/$artifact" ] && rm -f "$targetdir/$artifact"

golangbuilder=$PWD/.golangbuilder.sh
dockerfile=$targetdir/Dockerfile

trap 'echo "[*] Cleaning up"; rm -f "$golangbuilder"; [ "$BUILDMODE" -eq 1 ] && rm -rf "$dockerfile"; exit' EXIT

# create script to run inside of golang builder
cat > "$golangbuilder" <<EOF
#!/bin/sh
set -ex
cd \$GOPATH

if [ ! -d "src/github.com/docker/docker" ]; then
  # minimize download
  git clone --single-branch --depth 1 https://github.com/docker/docker src/github.com/docker/docker
fi

repo1=github.com/gliderlabs/logspout
repo2=github.com/rtoma/logspout-redis-logstash

# ensure we get the current logspout version
if [ ! -d "src/\$repo1" ]; then
  # not cached, so get fresh
  git clone --single-branch -b "$logspout_version" --depth 1 https://\$repo1 src/\$repo1
  # give detached head a name so we can check later
  cd src/\$repo1
  git checkout -b "$logspout_version"
  cd -
  # save file for later
  cp src/\$repo1/modules.go src/\$repo1/modules.go.bak
elif [ "\$(cd src/\$repo1 && git rev-parse --abbrev-ref HEAD | cut -d/ -f2-)" != "$logspout_version" ]; then
  # if already in cache but different version, rm and get required version
  rm -rf src/\$repo1
  git clone --single-branch -b "$logspout_version" --depth 1 https://\$repo1 src/\$repo1
  # give detached head a name so we can check later
  cd src/\$repo1
  git checkout -b "$logspout_version"
  cd -
  # save file for later
  cp src/\$repo1/modules.go src/\$repo1/modules.go.bak
else
  # use saved file to overwrite our custom file
  cp src/\$repo1/modules.go.bak src/\$repo1/modules.go
fi
# get deps
go get -v \$repo1

# always start clean, wether dev mode or not
rm -rf src/\$repo2
# in dev mode: mkdir and copy source from local repo
if [ "$DEVMODE" -eq 1 ]; then
  mkdir -p src/\$repo2
  cp -rp /localrepo/* src/\$repo2
else
  # not in dev mode: get our version tag from github
  git clone --single-branch -b "$app_version" --depth 1 https://\$repo2 src/\$repo2
fi
# get deps for build + testing
go get -v -t \$repo2

cat > src/\$repo1/modules.go <<EOM
package main
import (
  _ "github.com/gliderlabs/logspout/httpstream"
  _ "github.com/gliderlabs/logspout/routesapi"
  _ "github.com/rtoma/logspout-redis-logstash"
)
EOM

cd src/\$repo2
go test -v

CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /target/linux.bin -ldflags "-X main.Version=v${app_version}-${logspout_version}" github.com/gliderlabs/logspout
EOF

chmod a+x "$golangbuilder"

# exec builder
echo "[*] Running Golang builder to compile v$app_version ..."
echo "[*] Golang image used: $GOLANG_BUILDER_IMAGE"
docker run --rm \
  -v "$golangbuilder":/builder.sh:ro \
  -v "$PWD/.cache":/go/src \
  -v "$PWD":/localrepo:ro \
  -v "$targetdir":/target \
  "$GOLANG_BUILDER_IMAGE" /builder.sh
echo

if [ ! -e "$targetdir/$artifact" ]; then
  echo "Building artifact failed. Stopping here..."
  exit 2
fi

cat > "$dockerfile" <<EOF
FROM $BASE_IMAGE
COPY $artifact /$artifact
ENTRYPOINT ["/$artifact"]
EOF

if [ "$BUILDMODE" -eq 1 ]; then
  echo "[*] Building Docker image $IMAGE:$app_version ..."
  docker build -f "$dockerfile" -t "$IMAGE:$app_version" target/
  echo
  echo "[*] Built $IMAGE image:"
  docker images | grep "^$IMAGE"
else
  echo "[*] We're in manual build mode: no Docker image will be build"
  echo "Dockerfile: $dockerfile"
  echo "Artifact  : $targetdir/$artifact"
fi
echo
