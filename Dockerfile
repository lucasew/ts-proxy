FROM alpine:3.23@sha256:865b95f46d98cf867a156fe4a135ad3fe50d2056aa3f25ed31662dff6da4eb62

RUN apk add --no-cache ca-certificates iptables iproute2 ip6tables

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/ts-proxyd /usr/local/bin/ts-proxyd

ENTRYPOINT [ "/usr/local/bin/ts-proxyd" ]
