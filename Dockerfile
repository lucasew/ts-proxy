FROM alpine:3.24@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

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
