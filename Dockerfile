FROM golang:1.21-alpine AS build-env

WORKDIR /go/src/ts-proxy

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

ARG VERSION_LONG
ENV VERSION_LONG=$VERSION_LONG

ARG VERSION_GIT
ENV VERSION_GIT=$VERSION_GIT

RUN go build -v -o ts-proxyd ./cmd/ts-proxyd

FROM alpine:3.21

RUN apk add --no-cache ca-certificates iptables iproute2 ip6tables

COPY --from=build-env /go/src/ts-proxy/ts-proxyd /usr/local/bin

ENTRYPOINT [ "/usr/local/bin/ts-proxyd" ]

