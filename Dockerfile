ARG ALPINE_VERSION=3.22

# ----- Build Container --------
FROM golang:1.25.4-trixie AS builder

ENV GIT_SSH_COMMAND="ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no"

ADD . /build

WORKDIR /build

RUN git config --global url.ssh://git@github.com/.insteadOf https://github.com/
RUN --mount=type=ssh make build

# -- Production Container --------------------
# This needs to be a real OS container because the inotify stuff calls exec
FROM library/alpine:$ALPINE_VERSION
RUN apk add --update bind-tools

COPY --from=builder /build/logtailer /logtailer
COPY run.sh /run.sh

CMD ["/run.sh"]
