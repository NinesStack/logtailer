# -- Build Container -------------------------
FROM quay.io/shimmur/go-build-base:1.18.2 AS builder

ARG ALPINE_VERSION=3.17

# Switch workdir, otherwise we end up in /go (default)
WORKDIR /build

# Copy everything into build container
COPY . .

# Build the application
RUN go build

# -- Production Container --------------------
# This needs to be a real OS container because the inotify stuff calls exec
FROM alpine:3.17
RUN apk add --update bind-tools

COPY --from=builder /build/logtailer /logtailer
COPY run.sh /run.sh

CMD ["/run.sh"]
