# syntax=docker/dockerfile:1.7

ARG GO_IMAGE=golang:1.26-alpine
ARG ALPINE_IMAGE=alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build-base

ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache ca-certificates git
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY deploy/ ./deploy/
COPY internal/ ./internal/

ENV CGO_ENABLED=0

FROM build-base AS build-server
ARG VCS_REF=unknown
ARG VCS_BRANCH=unknown
ARG VCS_TREE_STATE=unknown
ARG BUILD_DATE=unknown
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w -X main.gitCommit=${VCS_REF} -X main.gitBranch=${VCS_BRANCH} -X main.gitTreeState=${VCS_TREE_STATE} -X main.buildTime=${BUILD_DATE}" \
      -o /out/rugram ./cmd/rugram

FROM build-base AS build-admin
RUN apk add --no-cache nodejs npm
WORKDIR /src/cmd/rugram-admin/web
RUN npm ci && npm run build
WORKDIR /src
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/rugram-admin ./cmd/rugram-admin

FROM ${ALPINE_IMAGE} AS runtime-base

ARG VCS_REF=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="rugram" \
      org.opencontainers.image.description="Telegram-like MTProto server" \
      org.opencontainers.image.source="https://github.com/yvafa0975-ux/RuGram-ServerPartTelegram" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}"

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 rugram \
    && adduser -S -D -H -u 10001 -G rugram rugram \
    && install -d -o rugram -g rugram -m 0750 /app /var/lib/rugram

COPY --chmod=0555 deploy/docker/docker-entrypoint.sh /usr/local/bin/rugram-container-entrypoint

WORKDIR /app
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/rugram-container-entrypoint"]

FROM runtime-base AS server
USER root
RUN apk add --no-cache ffmpeg openssl \
    && install -d -o rugram -g rugram -m 0750 \
      /var/lib/rugram/blobs \
      /var/lib/rugram/blob-staging \
      /var/lib/rugram/maptiles \
      /var/lib/rugram/livestream
# The wallet directory is deliberately NOT in the list above. It holds one
# 0600 JSON per account plus the AES-256-GCM master key, and NewFileStore
# refuses to open a state directory that is group- or world-accessible.
# Installing it 0750 alongside the others stopped the server at boot with
# "wallet state directory must have mode 0700".
RUN install -d -o rugram -g rugram -m 0700 /var/lib/rugram/wallet
COPY --from=build-server /out/rugram /usr/local/bin/rugram
COPY --chown=rugram:rugram data/langpack/ /usr/share/rugram/langpack/
USER 10001:10001
EXPOSE 2398 2400 2401 2599 12399/udp 12400/udp
CMD ["rugram"]

FROM server AS server-test
USER root
RUN install -d -o rugram -g rugram -m 0755 /usr/share/rugram/keys
COPY --chown=rugram:rugram --chmod=0444 deploy/docker/assets/test-server-rsa.pub /usr/share/rugram/keys/test-server-rsa.pub
COPY --chown=rugram:rugram --chmod=0444 deploy/docker/assets/test-server-rsa.pem.b64 /usr/share/rugram/keys/test-server-rsa.pem.b64
USER 10001:10001

FROM runtime-base AS admin
COPY --from=build-admin /out/rugram-admin /usr/local/bin/rugram-admin
EXPOSE 2600
CMD ["rugram-admin"]
