# --- First stage
FROM golang:1.14-buster AS build

ARG REPOSITORY=Lirt
ARG PLUGIN=velero-plugin-swift
ENV GOPROXY=https://proxy.golang.org

WORKDIR /go/src/github.com/${REPOSITORY}/${PLUGIN}
COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/${PLUGIN} .

# --- Second stage
FROM ubuntu:bionic
RUN mkdir /plugins
COPY --from=build /go/bin/${PLUGIN} /plugins/
USER nobody:nogroup
ENTRYPOINT ["/bin/bash", "-c", "cp /plugins/* /target/."]

