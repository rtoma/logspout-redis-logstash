#!/bin/sh
cat > ./Dockerfile.example <<DOCKERFILE
FROM gliderlabs/logspout:master
DOCKERFILE

cat > ./modules.go <<MODULES
package main
import (
  _ "github.com/gliderlabs/logspout/httpstream"
  _ "github.com/gliderlabs/logspout/routesapi"
  _ "github.com/BisnodeInformatics/logspout-redis-logstash"
)
MODULES

docker build -t bisnode/logspout-redis-logstash -f Dockerfile.example .

rm -f Dockerfile.example modules.go
