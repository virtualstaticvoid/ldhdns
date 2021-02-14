# Local Docker Host DNS - ldhdns

[![Build Status](https://travis-ci.org/virtualstaticvoid/ldhdns.svg?branch=master)](https://travis-ci.org/virtualstaticvoid/ldhdns)
[![](https://images.microbadger.com/badges/image/virtualstaticvoid/ldhdns.svg)](https://microbadger.com/images/virtualstaticvoid/ldhdns)
[![](https://images.microbadger.com/badges/version/virtualstaticvoid/ldhdns.svg)](https://microbadger.com/images/virtualstaticvoid/ldhdns)

A developer tool for providing DNS for Docker containers running on a local development host.

## Requirements

* Linux operating system (e.g. Ubuntu 20.04)
* [`systemd-resolved`][resolved] service (enabled and running)
* [`docker`][docker] (`>= 20.10`)
* [`docker-compose`][docker-compose] (`>= 1.27`)
* _optionally_, `make` for build tasks

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

Visit [hub.docker.com/r/virtualstaticvoid/ldhdns][docker-hub] for available image tags.

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

**Please inspect the code in the [this repository][ldhdns] and build the image yourself if you are concerned about security.**

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

*Note*: Make sure to use the same label key you provided in the `LDHDNS_SUBDOMAIN_LABEL` environment variable.
*Note*: Labels cannot be added to existing containers so you will need to re-create them to apply the label.

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

## Background

Consider a scenario in development where you are building a Single Page Web Application (SPA) and REST API, with a PostgreSQL database, with each service running in Docker containers on your local machine.

![](doc/example-before.svg)

A web browser connects to the Web Application and the REST API, and the API connects to the PostgreSQL database.

In development, to access these services there are number of options:

1. Map the container ports to host ports and access the services using `localhost` together with the host port number,
2. Obtain the IP address of each respective container and container port number,
3. Using domain names instead of IP addresses, adding them to your `/etc/hosts` to map each container IP address to a name.

Each of these methods have difficulties, short-comings and implications, such as when:

* Multiple services using the same port number.
* Steps needed to get the IP addresses of containers.
* Editing `/etc/hosts` requires root permissions.
* Manual updates to `/etc/hosts` needed when an IP address changes.
* Running containers with a static IP address so that `/etc/hosts` doesn't need to change.
* Using complicated scripts to inspect containers to get their IP addresses, write out configuration files and restart services.
* Different code paths to handle differences between development and deployed environments.
* Having to register your own domain name.
* Managing DNS records.
* Waiting for DNS updates to take effect.
* Portability issues for other developers on their machines when collaborating on projects.

Furthermore, host to container port mappings are typically used, so it could be `8080` to `80` for the Web Application and `8090` to `80` for the REST API. The SPA Web Application would therefore have to be configured to use `http://localhost:8090` to access the API. However the API connects directly to PostgreSQL so it would have to configured to use the PostgreSQL container name.

You may also want to run some ad-hoc SQL queries whilst debugging, so connecting a tool such as `psql` would require a further port mapping of `8432` to `5432`.

As you can see this setup gets messy and complicated quickly and isn't a great developer experience!

Now imagine adding SSL ports (`443`) so that you can debug under more production like conditions; the situation gets nasty fast. Don't even think about having more than one instance of a container, such as when using the `docker-compose up --scale` to add more container instances of a service!

## Solution

`ldhdns` provides a simple solution. It monitors running containers for labels which contain the domain name to use and configures and runs a lightweight DNS server. These domain names are dynamically resolveable on the host and from within containers, so that you can use the same fully qualified domain names in each scenario and use the _actual service ports_ just like in production.

In the above mentioned example, you could use `web`, `api` and `pgsql` as the subdomains for the respective containers. The Web Application URL would be `http://web.ldh.dns`, the REST API would be `http://api.ldh.dns` and the PostgreSQL service would be `pgsql.ldh.dns`.

![](doc/example-after.svg)

## Architecture

`ldhdns` consists of two services which are packaged in the same Docker container.

The following diagram illustrates the components which make up the solution, and how they interact with the host machine, the docker API, systemd-resolved and other applications such as a browser or psql.

![](doc/architecture.svg)

The controller creates and configures a Docker bridge network and configures `systemd-resolved`. It spawns the second service, which monitors the Docker API for when containers are started or stopped; creating and removing DNS records accordingly; and runs `dnsmasq` to resolve DNS queries for `A` (ipv4) and `AAAA` (ipv6) type records for the given domain.

## Building

Use `make` to build the `ldhdns` docker image.

## Debugging

You can query the DNS records to check that it works by using `dig`:

```
dig -t ANY web.ldh.dns

...
; <<>> DiG 9.16.1-Ubuntu <<>> -t A web.ldh.dns
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: ...
...

;; ANSWER SECTION:
web.ldh.dns.     15 IN  A 172.18.0.3
web.ldh.dns.     15 IN  AAAA fc00:f853:ccd:e793::3

...
```

And cross-checking with the IP addresses of each running container labelled with `dns.ldh/subdomain`:

```
docker ps --filter='label=dns.ldh/subdomain=web' --format "{{.ID}}" | \
  xargs docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}} [{{.GlobalIPv6Address}}]{{end}}' --

...
172.18.0.3 [fc00:f853:ccd:e793::3]
```




## Inspiration

* I got tired of running `docker ps` to figure out the container name, followed by `docker inspect` to get the IP address and then manually editing `/etc/hosts`.
* I couldn't come up with a consistent convention for mapping host to container ports. What comes after 8099?
* Finding IPv4 and IPv6 CIDR blocks which aren't in use so that static IP's can be used.
* Not being able to create SSL certificates for `*.xip.io` or `*.nip.io` domains.

## Credits

* [Configure systemd-resolved to use a specific DNS nameserver for a given domain][brasey]
* [How to configure systemd-resolved and systemd-networkd to use local DNS server for resolving local domains and remote DNS server for remote domains][stackexchange]
* [`dnsmasq`][dnsmasq]
* [`dnsmasq` Tips and Tricks][dnsmasq-tips]
* [github.com/programster/docker-dnsmasq][programster]
* [`systemd-resolved`][resolved]
* [github.com/jonathanio/update-systemd-resolved][jonathanio]

## License

MIT License. Copyright (c) 2020 Chris Stefano. See [LICENSE](LICENSE) for details.

<!-- links -->

[brasey]: https://gist.github.com/brasey/fa2277a6d7242cdf4e4b7c720d42b567#solution
[dnsmasq-tips]: https://www.linux.com/topic/networking/advanced-dnsmasq-tips-and-tricks/
[dnsmasq]: http://www.thekelleys.org.uk/dnsmasq/doc.html
[docker-compose]: https://docs.docker.com/compose/install
[docker-hub]: https://hub.docker.com/repository/docker/virtualstaticvoid/ldhdns
[docker]: https://docs.docker.com/get-started
[jonathanio]: https://github.com/jonathanio/update-systemd-resolved
[ldhdns]: https://github.com/virtualstaticvoid/ldhdns
[programster]: https://github.com/programster/docker-dnsmasq
[resolved]: https://www.freedesktop.org/wiki/Software/systemd/resolved/
[stackexchange]: https://unix.stackexchange.com/a/442599
