FROM golang:1.26.0-alpine@sha256:d4c4845f5d60c6a974c6000ce58ae079328d03ab7f721a0734277e69905473e5 AS build-env

WORKDIR /go/src/ts-proxy

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

ARG VERSION_LONG
ENV VERSION_LONG=$VERSION_LONG

ARG VERSION_GIT
ENV VERSION_GIT=$VERSION_GIT

RUN go build -v -o ts-proxyd ./cmd/ts-proxyd

FROM alpine:3.23@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62

RUN apk add --no-cache ca-certificates iptables iproute2 ip6tables

COPY --from=build-env /go/src/ts-proxy/ts-proxyd /usr/local/bin

ENTRYPOINT [ "/usr/local/bin/ts-proxyd" ]

