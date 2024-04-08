# syntax=docker/dockerfile:1.3

# --- First stage
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS build

ARG REPOSITORY=Lirt
ARG PLUGIN=velero-plugin-for-openstack
ARG PKG=github.com/Lirt/velero-plugin-for-openstack
ARG VERSION=0.0.0
ARG GIT_SHA=nil

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
RUN export LDFLAGS="-X ${PKG}/src/utils.Version=${VERSION} -X ${PKG}/src/utils.GitSHA=${GIT_SHA}" \
    export GOARM=$( echo "${GOARM}" | cut -c2-) && \
    CGO_ENABLED=0 \
      go build \
      -ldflags "${LDFLAGS}" \
      -o /go/bin/${PLUGIN} \
      .

# --- Second stage
FROM alpine:3
RUN mkdir /plugins
COPY --from=build /go/bin/${PLUGIN} /plugins/
USER nobody:nogroup
ENTRYPOINT ["cp", "-rT", "/plugins/", "/target/"]
