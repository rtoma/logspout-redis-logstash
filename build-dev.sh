#!/bin/sh

if [ ! -d src/github.com/gliderlabs/logspout ]; then
    git clone --depth 1 https://github.com/gliderlabs/logspout src/github.com/gliderlabs/logspout
fi

mkdir -p src/github.com/rtoma/logspout-redis-logstash
cp redis.go src/github.com/rtoma/logspout-redis-logstash

cat > ./src/github.com/gliderlabs/logspout/modules.go <<MODULES
package main
import (
  _ "github.com/gliderlabs/logspout/httpstream"
  _ "github.com/gliderlabs/logspout/routesapi"
  _ "github.com/rtoma/logspout-redis-logstash"
)
MODULES

docker run --net host --name logspout-builder --rm \
  -e DEBUG=X \
  -e REDIS_DOCKER_HOST=$(hostname) \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v $PWD/src:/go/src -w /go/src/github.com/gliderlabs/logspout \
  golang:1.5 sh -c "\
go get -v
go build -v
trap 'kill -TERM \$PID' TERM INT
./logspout \"$*\" &
PID=\$!
wait \$PID
trap - TERM INT
wait \$PID"
