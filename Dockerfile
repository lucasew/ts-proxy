FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

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
