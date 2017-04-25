#!/bin/sh

set -e

go fmt $(go list | grep -v /vendor/)

glide install
CGO_ENABLED=0 go build -a

docker build -t mopsalarm/pr0gramm-thumby .
docker push mopsalarm/pr0gramm-thumby
