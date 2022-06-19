# build image
FROM golang:1.15-buster AS builder

RUN mkdir -p /go/src/go.virtualstaticvoid.com/ldhdns
WORKDIR /go/src/go.virtualstaticvoid.com/ldhdns

COPY go.* ./
RUN go mod download

ARG VERSION

COPY *.go .
COPY cmd/ cmd/
COPY internal/ internal
RUN go build -ldflags="-X go.virtualstaticvoid.com/ldhdns/cmd.Version=$VERSION" -o /go/bin/ldhdns

# final image
FROM debian:buster

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update -qq \
 && apt-get install -qy \
      curl \
      dnsmasq \
      dumb-init \
      xz-utils \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

ARG S6_VERSION

RUN curl -sSL "https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-noarch.tar.xz" | tar -Jxpf - -C / \
 && curl -sSL "https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-$(uname -m).tar.xz" | tar -Jxpf - -C / \
 && curl -sSL "https://github.com/just-containers/s6-overlay/releases/download/${S6_VERSION}/s6-overlay-symlinks-noarch.tar.xz" | tar -Jxpf - -C /

COPY --from=builder /go/bin/ldhdns /usr/bin/

COPY services /etc/services.d/
COPY dnsmasq /etc/ldhdns/dnsmasq/

COPY docker-entrypoint.sh /usr/bin/docker-entrypoint.sh
RUN chmod +x /usr/bin/docker-entrypoint.sh

ENV DOCKER_HOST=unix:///tmp/docker.sock
ENV DNSMASQ_HOSTSDIR=/etc/ldhdns/dnsmasq/hosts.d
ENV DNSMASQ_PIDFILE=/var/run/dnsmasq.pid
ENV DNSMASQ_LOCAL_TTL=15

# configuration
ENV LDHDNS_NETWORK_ID=ldhdns
ENV LDHDNS_DOMAIN_SUFFIX=ldh.dns
ENV LDHDNS_SUBDOMAIN_LABEL=dns.ldh/subdomain
ENV LDHDNS_CONTAINER_NAME=ldhdns

ENTRYPOINT ["/usr/bin/dumb-init", "--", "docker-entrypoint.sh"]
