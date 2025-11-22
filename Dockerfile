FROM golang:1.25-alpine@sha256:d3f0cf7723f3429e3f9ed846243970b20a2de7bae6a5b66fc5914e228d831bbb AS build-env

WORKDIR /go/src/ts-proxy

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

ARG VERSION_LONG
ENV VERSION_LONG=$VERSION_LONG

ARG VERSION_GIT
ENV VERSION_GIT=$VERSION_GIT

RUN go build -v -o ts-proxyd ./cmd/ts-proxyd

FROM alpine:3.22

RUN apk add --no-cache ca-certificates iptables iproute2 ip6tables

COPY --from=build-env /go/src/ts-proxy/ts-proxyd /usr/local/bin

ENTRYPOINT [ "/usr/local/bin/ts-proxyd" ]

