FROM alpine:3.24@sha256:3f4d6b32e26703c6082839234f43769d5046b3ac7c910033911d1e71d8cd630a

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
