#!/bin/sh
cat > ./Dockerfile.example <<DOCKERFILE
FROM gliderlabs/logspout:master

ENV KAFKA_COMPRESSION_CODEC snappy
DOCKERFILE

cat > ./modules.go <<MODULES
package main
import (
  _ "github.com/gliderlabs/logspout/httpstream"
  _ "github.com/gliderlabs/logspout/routesapi"
  _ "github.com/rtoma/logspout-redis-logstash"
)
MODULES

docker build -t rtoma/example-logspout -f Dockerfile.example .
