# syntax=docker/dockerfile:1.3

# --- First stage
FROM --platform=$BUILDPLATFORM golang:1.16-alpine AS build

ARG REPOSITORY=Lirt
ARG PLUGIN=velero-plugin-swift

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

ARG GOOS=linux
ARG GOARCH=amd64

ENV GOOS=${TARGETOS}
ENV GOARCH=${TARGETARCH}
ENV GOARM=${TARGETVARIANT}

ENV GOPROXY=https://proxy.golang.org

WORKDIR /go/src/github.com/${REPOSITORY}/${PLUGIN}
COPY . .
RUN export GOARM=$( echo "${GOARM}" | cut -c2-) && \
    CGO_ENABLED=0 \
      go build \
      -o /go/bin/${PLUGIN} \
      .

# --- Second stage
FROM alpine:3
RUN mkdir /plugins
COPY --from=build /go/bin/${PLUGIN} /plugins/
USER nobody:nogroup
ENTRYPOINT ["cp", "-rT", "/plugins/", "/target/"]
