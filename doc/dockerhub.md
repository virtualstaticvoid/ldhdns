# Local Docker Host DNS - ldhdns

A developer tool for providing DNS for Docker containers running on a local development host.

Please see [github.com/virtualstaticvoid/ldhdns][ldhdns] for more details.

## Requirements

* Linux operating system (e.g. Ubuntu)
* Docker
* [`systemd-resolved`][resolved] service enabled and running
* Optionally, a domain name that you own

## Usage

### The Controller

Start the controller, attaching it to the Docker host network, as follows:

**Security Note:** The controller needs to mount the Docker socket so that it can consume the Docker API and it is run with the `apparmor=unconfined` security option and mounts the SystemBus Socket so that it is able to configure `systemd-resolved` dynamically.

**Please inspect the code [in this repository][ldhdns] and build the `ldhdns` container yourself if you are concerned about security.**

```
docker run \
  --detach \
  --network host \
  --security-opt "apparmor=unconfined" \
  --volume "/var/run/docker.sock:/tmp/docker.sock" \
  --volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
  --restart always \
  virtualstaticvoid/ldhdns:latest
```

Optionally, the network ID, domain name suffix and subdomain label can be configured with environment variables:

* `LDHDNS_NETWORK_ID` for docker network name to use. The default is `ldhdns`.
* `LDHDNS_DOMAIN_SUFFIX` for domain name suffix to use. The default is `ldh.dns`.
* `LDHDNS_SUBDOMAIN_LABEL` for label used by containers. The default is `dns.ldh/subdomain`.

**Tip:** If you are using a real domain name, be sure to use a subdomain on the TLD, such as `ldh`, to avoid any clashes with it's public DNS. E.g. Use `ldh.yourdomain.com` on your local development host.

### Your Containers

To make containers resolvable, add the label "`dns.ldh/subdomain=<subdomain>`" with the desired subdomain to them.

This subdomain will be prepended to the domain name in the `LDHDNS_DOMAIN_SUFFIX` environment variable to form a fully qualified domain name.

Apply the label to a container using the command line:

```
docker run -it --label "dns.ldh/subdomain=foo" nginx
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

Make sure to use the same label key you provided in the `LDHDNS_SUBDOMAIN_LABEL` environment variable.

*Note*: Labels cannot be added to existing containers so you will need to re-create them to apply the label.

## License

MIT License. Copyright (c) 2020 Chris Stefano. See [LICENSE][license] for details.

<!-- links -->

[ldhdns]: https://github.com/virtualstaticvoid/ldhdns
[license]: https://github.com/virtualstaticvoid/ldhdns/blob/master/LICENSE
[resolved]: https://www.freedesktop.org/wiki/Software/systemd/resolved/
