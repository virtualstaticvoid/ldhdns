# Local Docker Host DNS - ldhdns

A tool to provide DNS for docker containers running on a single host.

## Usage

_TBC_

Start the controller docker container on the host network.

```
docker run \
  --detach \
  --network host \
  --restart always \
  --security-opt "apparmor=unconfined" \
  --env LDHDNS_DOMAIN_SUFFIX=ldh.dns \
  --volume "/var/run/docker.sock:/tmp/docker.sock" \
  --volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
  virtualstaticvoid/ldhdns:0.1.0
```

Add the `dns.ldh/subdomain` label to any other non-host networked container with the desired sub-domain.

E.g.

```
docker run -it --rm --label "dns.ldh/subdomain=foo" ubuntu:20.04 /bin/bash
```

Now you can access the container using a DNS name.

```
dig -t A foo.ldh.dns

; <<>> DiG 9.16.1-Ubuntu <<>> -t A foo.ldh.dns
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 56479
;; flags: qr rd ra; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 1

;; OPT PSEUDOSECTION:
; EDNS: version: 0, flags:; udp: 65494
;; QUESTION SECTION:
;foo.ldh.dns.     IN  A

;; ANSWER SECTION:
foo.ldh.dns.    600 IN  A 172.17.0.2

;; Query time: 0 msec
;; SERVER: 127.0.0.53#53(127.0.0.53)
;; WHEN: Sat Jul 25 21:31:19 BST 2020
;; MSG SIZE  rcvd: 56
```

## Configuration

_TBC_

## Building

_TBC_

```
make
```

## License

MIT License. Copyright (c) 2020 Chris Stefano. See [LICENSE](LICENSE) for details.
