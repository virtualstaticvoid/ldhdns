# Local Docker Host DNS - ldhdns

A developer tool for providing DNS for Docker containers running on a local development host.

Please see [github.com/virtualstaticvoid/ldhdns][ldhdns] for more details.

## Requirements

* Linux operating system (e.g. Ubuntu 20.04)
* [`systemd-resolved`][resolved] service (enabled and running)
* [`docker`][docker] (`>= 20.10`)

## Usage

### The Controller

Start the controller, attaching it to the Docker host network, as follows:

**Security Note:** The container mounts the Docker socket so that it can consume the Docker API and it is run with the `apparmor=unconfined` security option and mounts the SystemBus socket so that it is able to configure `systemd-resolved` dynamically.

```
docker run \
  --name ldhdns \
  --detach \
  --network host \
  --security-opt "apparmor=unconfined" \
  --volume "/var/run/docker.sock:/tmp/docker.sock" \
  --volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
  --restart unless-stopped \
  virtualstaticvoid/ldhdns:latest
```

Additionally, the network ID, domain name suffix and subdomain label can be configured with environment variables:

* `LDHDNS_NETWORK_ID` for docker network name to use. The default is `ldhdns`.
* `LDHDNS_DOMAIN_SUFFIX` for domain name suffix to use. The default is `ldh.dns`.
* `LDHDNS_SUBDOMAIN_LABEL` for label used by containers. The default is `dns.ldh/subdomain`.

**Tip:** If you are using a real domain name, be sure to use a subdomain such as `ldh` to avoid any clashes with it's public DNS.

You can provide your domain name via the `LDHDNS_DOMAIN_SUFFIX` environment variable as follows:

```
docker run \
  --name ldhdns \
  --env LDHDNS_DOMAIN_SUFFIX=ldh.example.com \
  --detach \
  --network host \
  --security-opt "apparmor=unconfined" \
  --volume "/var/run/docker.sock:/tmp/docker.sock" \
  --volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
  --restart unless-stopped \
  virtualstaticvoid/ldhdns:latest
```

**Please inspect the [code][ldhdns] and build the image yourself if you are concerned about security.**

### Your Containers

To make containers resolvable, add the label "`dns.ldh/subdomain=<subdomain>`" with the desired subdomain to use.

This subdomain will be prepended to the domain name in the `LDHDNS_DOMAIN_SUFFIX` environment variable to form a fully qualified domain name.

To apply the label to a container using the command line:

```
docker run -it --rm --label "dns.ldh/subdomain=foo" nginx
```

Or with Docker Compose:

```
# docker-compose.yml
services:
  web:
    image: nginx
    labels:
      "dns.ldh/subdomain": "foo"
```

*Note*: Make sure to use the same label key you provided in the `LDHDNS_SUBDOMAIN_LABEL` environment variable. Labels cannot be added to existing containers so you will need to re-create them to apply the label.

Now the subdomain will be resolvable locally to the container IP address.

```
$ dig -t A foo.ldh.dns

; <<>> DiG 9.16.1-Ubuntu <<>> -t A foo.ldh.dns
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 61163
...

;; ANSWER SECTION:
foo.ldh.dns.    15  IN  A 172.17.0.2

...
```

And you can go ahead and consume the service.

```
$ curl http://foo.ldh.dns

<!DOCTYPE html>
<html>
...
<h1>Welcome to nginx!</h1>
...
</html>
```

## License

MIT License. Copyright (c) 2020 Chris Stefano. See [LICENSE][license] for details.

<!-- links -->

[docker]: https://docs.docker.com/get-started
[ldhdns]: https://github.com/virtualstaticvoid/ldhdns
[license]: https://github.com/virtualstaticvoid/ldhdns/blob/master/LICENSE
[resolved]: https://www.freedesktop.org/wiki/Software/systemd/resolved/
