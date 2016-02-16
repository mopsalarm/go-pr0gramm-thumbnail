FROM mopsalarm/ffmpeg-pidunu
COPY /go-pr0gramm-thumbnail /
EXPOSE 8080
ENTRYPOINT ["/pidunu", "/go-pr0gramm-thumbnail", "--path=/"]
