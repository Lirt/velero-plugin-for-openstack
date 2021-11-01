# --- First stage
FROM golang:1.16-alpine AS build

ARG REPOSITORY=Lirt
ARG PLUGIN=velero-plugin-swift
ARG GOOS=linux
ARG GOARCH=amd64

ENV GOPROXY=https://proxy.golang.org

WORKDIR /go/src/github.com/${REPOSITORY}/${PLUGIN}
COPY . .
RUN export GOOS=${GOOS}; \
    export GOARCH=${GOARCH}; \
    CGO_ENABLED=0 \
      go build \
      -o /go/bin/${PLUGIN} \
      .

# --- Second stage
FROM --platform=${GOOS}/${GOARCH} alpine:3
RUN mkdir /plugins
COPY --from=build /go/bin/${PLUGIN} /plugins/
USER nobody:nogroup
ENTRYPOINT ["cp", "-rT", "/plugins/", "/target/"]
