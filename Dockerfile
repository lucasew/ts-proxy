FROM alpine:3.23@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11

RUN apk add --no-cache ca-certificates iptables iproute2 ip6tables

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/ts-proxyd /usr/local/bin/ts-proxyd

ENTRYPOINT [ "/usr/local/bin/ts-proxyd" ]

