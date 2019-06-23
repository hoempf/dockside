FROM alpine:3.8
RUN apk update && apk add inotify-tools

CMD while true; do inotifywait /test && echo "change!"; done