# Local Docker Host DNS - ldhdns

[![Build Status](https://travis-ci.org/virtualstaticvoid/ldhdns.svg?branch=master)](https://travis-ci.org/virtualstaticvoid/ldhdns)
[![](https://images.microbadger.com/badges/image/virtualstaticvoid/ldhdns.svg)](https://microbadger.com/images/virtualstaticvoid/ldhdns)
[![](https://images.microbadger.com/badges/version/virtualstaticvoid/ldhdns.svg)](https://microbadger.com/images/virtualstaticvoid/ldhdns)

A developer tool for providing DNS for Docker containers running on a local development host.

## Why?

Consider a scenario where you have a Web Application (SPA), a REST API and PostgreSQL each running within Docker containers on your local development machine, wuth the HTTP services are accessible on port `80` and the PostgreSQL service on port `5432`.

To access these services locally you will need to either:

1. Map the container ports to host ports and access services using `localhost` together with the host port number,
2. Use the IP address of the respective container together with the container port number,
3. Manually add domain names to your `/etc/hosts` file for each container IP address needed.

Each of these methods have short-comings and/or issues, such as when:

* Multiple services use the same port numbers.
* Manually figuring out the IP address of containers.
* Manually editing `/etc/hosts`.
* Having to update `/etc/hosts` when an IP address changes.
* Running containers with a static IP address so that `/etc/hosts` doesn't need to change.
* Using complicated scripts to inspect containers to get their IP addresses and/or scripts to make configurations.
* More application configuration to run in both development and deployed environments without code differences.
* Portability issues for other developers on their machines when collaborating on projects.

In the above mentioned example, to access the SPA website with a browser, a typical host to container port mapping could be `8080` to `80` and a mapping of `8090` to `80` for the REST API, which would require the SPA application to be configured to use `http://localhost:8090` when using the API. You may also want to run some ad-hoc SQL queries whilst debugging, so connecting a tool such as `psql` would require a further port mapping of say `85432` to `5432` for PostgreSQL.

As you can see this setup gets complicated quickly and isn't a great developer experience!

Now imagine adding SSL ports (`443`) so that you can debug under more production like conditions; the situation gets nasty fast. Don't even think about having more than one instance of a container, such as when using the `docker-compose up --scale` command!

## Solution

`ldhdns` provides a simple solution.

You configure `ldhdns` with the domain name to use, such as 'ldh.dns' or use a real domain which you own, and then add a label for the sub-domain to each of the containers you want to use and `ldhdns` will make them dynamically resolveable on the development machine so that you can use a fully qualified domain name and the _actual service ports_ just like in production.

In the above mentioned example, you could associate `web`, `api` and `pgsql` as sub-domains for the respective containers and then have the SPA website configured by convention to use `api` as the sub-domain of the top-level domain for accessing the REST API.

If your domain were `ldh.dns` the website URL would be `http://web.ldh.dns` and the `psql` host would be `pgsql.ldh.dns`, or if you used a real domain such as `ldh.yourdomain.com` the URL would be `http://web.ldh.yourdomain.com` and you could use LetsEncrypt to create a wildcard SSL certificate for `*.ldh.yourdomain.com` so that you can develop with a more production like setup.

No need for port mappings or needing to know the IP addresses of containers, no manual editing of `/etc/hosts` or having funky configuration to manage differences between development and deployed environments.

Happy developer!

## Requirements

* Linux operating system (e.g. Ubuntu)
* Docker
* [`systemd-resolved`][resolved] service enabled and running
* Optionally, a domain name that you own

## Usage

### The Controller

The `ldhdns` controller application is packaged as a Docker container. It manages DNS using a Docker bridge network and `systemd-resolved` together with a second container which handles Docker container start and stop events.

**Security Note:** The controller needs to mount the Docker socket so that it can consume the Docker API and it is run with the `apparmor=unconfined` security option and mounts the SystemBus Socket so that it is able to configure `systemd-resolved` dynamically.

**Please read the code in this repository and build the `ldhdns` container yourself if you are concerned about security.**

Start the controller, attaching it to the Docker host network, as follows:

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

Optionally, the network ID, domain name suffix and sub-domain label can be configured with environment variables:

* `LDHDNS_NETWORK_ID` for docker network name to use. The default is `ldhdns`.
* `LDHDNS_DOMAIN_SUFFIX` for domain name suffix to use. The default is `ldh.dns`.
* `LDHDNS_SUBDOMAIN_LABEL` for label used by containers. The default is `dns.ldh/subdomain`.

**Tip:** If you are using a real domain name, be sure to use a sub-domain on the TLD, such as `ldh`, to avoid any clashes with it's public DNS. E.g. Use `ldh.yourdomain.com` on your local development host.

### Your Containers

To make containers resolvable, add the label "`dns.ldh/subdomain=<sub-domain>`" with the desired sub-domain to them.

This sub-domain will be prepended to the domain name in the `LDHDNS_DOMAIN_SUFFIX` environment variable to form a fully qualified domain name.

To apply the label to a container, for example from the command line:

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

*Note*: Labels cannot be added to existing containers so you will need to re-create them to apply the label.

### Consuming

Now you can access the respective container using it's fully qualified domain name.

For example, in visiting `http://foo.ldh.dns` with your browser or using `curl` from the command line:

```
curl -v http://foo.ldh.dns
```

Or `psql` connecting to a PostgreSQL container:

```
psql --host foo.ldh.dns
```

### Building

Use `make` to build the `ldhdns` docker image.

### Debugging

You can query the DNS records to check that it works by using `dig`:

```
dig -t ANY foo.ldh.dns

...
; <<>> DiG 9.16.1-Ubuntu <<>> -t A foo.ldh.dns
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: ...
...

;; ANSWER SECTION:
foo.ldh.dns.    600 IN  A 172.18.0.3
foo.ldh.dns.    600 IN  AAAA fc00:f853:ccd:e793::3
...
```

And cross-checking with the IP addresses of each running container labelled with `dns.ldh/subdomain`:

```
docker ps --filter='label=dns.ldh/subdomain=foo' --format "{{.ID}}" | \
  xargs docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}} [{{.GlobalIPv6Address}}]{{end}}' --

...
172.18.0.3 [fc00:f853:ccd:e793::3]
```

## How It Works

_TBC_

The following diagram illustrates the components which make up the solution, and how they interact with the host machine, the docker API, systemd-resolved and other applications such as a browser or psql.

![](doc/diagram.svg)

## Inspiration

* I got tired of running `docker ps` to figure out the container name, followed by `docker inspect` to get the IP address and then manually editing `/etc/hosts`.
* I couldn't come up with a consistent convention for mapping host to container ports. What comes after 8099?

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
[jonathanio]: https://github.com/jonathanio/update-systemd-resolved
[programster]: https://github.com/programster/docker-dnsmasq
[resolved]: https://www.freedesktop.org/wiki/Software/systemd/resolved/
[stackexchange]: https://unix.stackexchange.com/a/442599
