FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

RUN apk add --no-cache ca-certificates iptables iproute2 ip6tables \
	&& addgroup -g 65532 -S tsproxy \
	&& adduser -u 65532 -S -D -H -h /var/lib/ts-proxy -G tsproxy tsproxy \
	&& mkdir -p /var/lib/ts-proxy \
	&& chown tsproxy:tsproxy /var/lib/ts-proxy

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/ts-proxyd /usr/local/bin/ts-proxyd

USER 65532:65532

ENTRYPOINT [ "/usr/local/bin/ts-proxyd" ]

CMD [ "server" ]
