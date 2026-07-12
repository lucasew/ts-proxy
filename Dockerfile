FROM alpine:3.24@sha256:a2d49ea686c2adfe3c992e47dc3b5e7fa6e6b5055609400dc2acaeb241c829f4

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
