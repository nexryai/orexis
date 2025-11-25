FROM golang:1-alpine
RUN apk --no-cache update && apk --no-cache upgrade \
    && apk --no-cache add git nushell curl jq ca-certificates openssh-client \
    && addgroup -g 630 dev \
	&& adduser -u 630 -G dev -D -h /home/dev --shell /usr/bin/nu dev
