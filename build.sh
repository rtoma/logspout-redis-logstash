#!/bin/sh

GOLANG_BUILDER_IMAGE=golang:1.7
LOGSPOUT_BRANCH=master
LOGSPOUT_REDIS_LOGSTASH_BRANCH=master
BASE_IMAGE=scratch
IMAGE_VERSION=latest
IMAGE=artifacts.ath.bskyb.com:5001/olisipo/logspout-redis-logstash

usage() {
  echo "Usage: $0 [-c] [-d] [-n] [-b <base image>] [-g <golang builder image>] [-v <version>] <logspout redis logstash branch> [<logspout branch>]"
  echo
  echo "Parameters:"
  echo "   -c           : Don't use cache for Golang sources (will slow down building)."
  echo "   -d           : Enable development mode. Local sourcefile will be used instead of Github."
  echo "   -n           : Skip building of Docker image, to allow manual building."
  echo "   -b <image>   : Set different Docker base image."
  echo "   -g <image>   : Set different Golang builder image."
  echo "   -v <version> : Set the image version (default is latest)"
  exit "${1:-0}"
}


DEVMODE=0
BUILDMODE=1
USECACHE=1
while getopts "cdnhg:b:v:" opt; do
  case $opt in
    c ) USECACHE=0;;
    d ) DEVMODE=1;;
    n ) BUILDMODE=0;;
    g ) GOLANG_BUILDER_IMAGE="$OPTARG";;
    b ) BASE_IMAGE="$OPTARG";;
    v ) IMAGE_VERSION="$OPTARG";;
    h ) usage;;
    \?) usage 1;;
    : ) usage 1;;
  esac
done
shift "$((OPTIND - 1))"

LOGSPOUT_REDIS_LOGSTASH_BRANCH=$1
LOGSPOUT_BRANCH=$2
if [ -z "$LOGSPOUT_REDIS_LOGSTASH_BRANCH" ]; then
  echo "Missing <logspout redis logstash branch>"
  usage 1
fi
if [ -z "$LOGSPOUT_BRANCH" ]; then
  echo "Missing <logspout branch>"
  usage 1
fi

if [ "$DEVMODE" -eq 1 ]; then
  echo "[*] Running in DEV mode - using local sourcefile"
  IMAGE_VERSION="${IMAGE_VERSION}-dev"
fi

set -e

# setup clean target dir
targetdir=$PWD/target
[ ! -d "$targetdir" ] && mkdir -p "$targetdir"

if [ "$USECACHE" -eq 1 ]; then
  echo "[*] Using local cachedir for Golang sources"
  cachedir=$PWD/.cache
  [ ! -d "$cachedir" ] && mkdir -p "$cachedir"
  docker_cacheopts="-v $cachedir:/go/src"
else
  echo "[*] Not using local cachedir for Golang sources (this will slow down builds)"
  docker_cacheopts=
fi

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

# fix for internal gobuilder image with bad src
#rm -rf src/golang.org/x/net

if [ ! -d "src/github.com/docker/docker" ]; then
  # minimize download
  git clone --single-branch --depth 1 https://github.com/docker/docker src/github.com/docker/docker
fi

repo1=github.com/strava/logspout
repo2=github.com/rtoma/logspout-redis-logstash

# ensure we get the current logspout version
if [ ! -d "src/\$repo1" ]; then
  # not cached, so get fresh
  git clone https://\$repo1 src/\$repo1
  cd src/\$repo1
  git checkout "$LOGSPOUT_BRANCH"
  cd -
  # save file for later
  cp src/\$repo1/modules.go src/\$repo1/modules.go.bak
elif [ "\$(cd src/\$repo1 && git rev-parse --abbrev-ref HEAD | cut -d/ -f2-)" != "$LOGSPOUT_BRANCH" ]; then
  # if already in cache but different version, rm and get required version
  rm -rf src/\$repo1
  git clone https://\$repo1 src/\$repo1
  cd src/\$repo1
  git checkout "$LOGSPOUT_BRANCH"
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
  git clone https://\$repo2 src/\$repo2
  cd src/\$repo2
  git checkout "$LOGSPOUT_REDIS_LOGSTASH_BRANCH"
  cd -
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

CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /target/linux.bin -ldflags "-X main.Version=${IMAGE_VERSION}-${LOGSPOUT_REDIS_LOGSTASH_BRANCH}.${LOGSPOUT_BRANCH}" github.com/gliderlabs/logspout
EOF

chmod a+x "$golangbuilder"

# exec builder
echo "[*] Running Golang builder to compile $IMAGE_VERSION ..."
echo "[*] Golang image used: $GOLANG_BUILDER_IMAGE"
docker run --rm \
  -v "$golangbuilder":/builder.sh:ro \
  -v "$PWD":/localrepo:ro \
  -v "$targetdir":/target \
  -e "http_proxy=${http_proxy:-}" \
  -e "https_proxy=${https_proxy:-}" \
  -e "HTTP_PROXY=$HTTP_PROXY" \
  -e "HTTPS_PROXY=$HTTPS_PROXY" \
  $docker_cacheopts \
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
  echo "[*] Building Docker image $IMAGE:$IMAGE_VERSION ..."
  docker build -f "$dockerfile" \
    --build-arg "http_proxy=$http_proxy" \
    --build-arg "https_proxy=$https_proxy" \
    --build-arg "HTTP_PROXY=$HTTP_PROXY" \
    --build-arg "HTTPS_PROXY=$HTTPS_PROXY" \
    -t "$IMAGE:$IMAGE_VERSION" target/
  echo
  echo "[*] Built $IMAGE image:"
  docker images | grep "^$IMAGE"
else
  echo "[*] We're in manual build mode: no Docker image will be build"
  echo "Dockerfile: $dockerfile"
  echo "Artifact  : $targetdir/$artifact"
fi
echo
