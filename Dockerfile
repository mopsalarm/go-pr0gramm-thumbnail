FROM alpine:3.5

EXPOSE 8080

# install init process
ADD https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 /dumb-init
RUN chmod a+x /dumb-init

# install a current ffmpeg static build
RUN apk add --no-cache tar xz curl \
 && URL=https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-64bit-static.tar.xz \
 && curl  $URL | xz -d | tar -x -C /usr/bin --strip-components=1 \
 && rm -f /usr/bin/ffmpeg-10bit /usr/bin/ffserver \
 && apk del xz curl tar

# install our binary
COPY /go-pr0gramm-thumbnail /

ENTRYPOINT ["/dumb-init", "/go-pr0gramm-thumbnail", "--path=/tmp"]
