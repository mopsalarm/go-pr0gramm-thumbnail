FROM golang:1.12.6-alpine3.9 as builder

RUN apk add --no-cache git

ENV GO111MODULE=on
ENV PACKAGE github.com/mopsalarm/go-pr0gramm-thumbnail
WORKDIR $GOPATH/src/$PACKAGE/

COPY go.mod go.sum ./
RUN go mod download

ENV CGO_ENABLED=0

COPY . .
RUN go build -v -ldflags="-s -w" -o /service .


FROM alpine:3.9

EXPOSE 8080

# install init process
ADD https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 /dumb-init
RUN chmod a+x /dumb-init

# install a current ffmpeg static build
RUN apk add --no-cache tar xz curl \
 && URL=https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz \
 && curl $URL | xz -d | tar -x -C /usr/bin --strip-components=1 \
 && rm -f /usr/bin/ffmpeg-10bit /usr/bin/ffserver \
 && apk del xz curl tar

RUN apk add --no-update ca-certificates

# install our binary
COPY --from=builder /service /go-pr0gramm-thumbnail

ENTRYPOINT ["/dumb-init", "/go-pr0gramm-thumbnail", "--path=/tmp"]
