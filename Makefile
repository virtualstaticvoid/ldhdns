# variables
IMAGE:=ldhdns
DOCKER_REPO:=virtualstaticvoid
VERSION?=latest

NETWORK_ID?=ldh.dns
DOMAIN_SUFFIX?=ldh.dns
SUBDOMAIN_LABEL?=dns.ldh/subdomain

export

# default build target
all::

.PHONY: all
all:: build

.PHONY: build
build:

	docker build \
		--tag $(IMAGE):$(VERSION) \
		--tag $(DOCKER_REPO)/$(IMAGE):$(VERSION) \
		--build-arg VERSION=$(VERSION) \
		.

.PHONY: debug
debug:

  # run test controller for foo*
	docker run \
		--detach \
		--rm \
		--label "$(SUBDOMAIN_LABEL)=foo" \
		nginx:stable

  # run controller interactively (Ctrl+C to quit)
	docker run \
		--name ldhdnsdebug \
		--rm \
		--interactive \
		--tty \
		--network host \
		--env "LDHDNS_NETWORK_ID=$(NETWORK_ID)" \
		--env "LDHDNS_DOMAIN_SUFFIX=$(DOMAIN_SUFFIX)" \
		--env "LDHDNS_SUBDOMAIN_LABEL=$(SUBDOMAIN_LABEL)" \
		--security-opt "apparmor=unconfined" \
		--volume "/var/run/docker.sock:/tmp/docker.sock" \
		--volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
		$(IMAGE):$(VERSION)

	# clean up
	@docker ps --filter='label=dns.ldh/controller-name=ldhdnsdebug' --format "{{.ID}}" | xargs docker stop || true
	@docker ps --filter='label=$(SUBDOMAIN_LABEL)=foo' --format "{{.ID}}" | xargs docker stop || true
	@docker network rm $(NETWORK_ID) || true

.PHONY: publish
publish:

	docker push $(DOCKER_REPO)/$(IMAGE):$(VERSION)

.PHONY: install
install:

	docker run \
		--name ldhdns \
		--detach \
		--network host \
		--env "LDHDNS_NETWORK_ID=$(NETWORK_ID)" \
		--env "LDHDNS_DOMAIN_SUFFIX=$(DOMAIN_SUFFIX)" \
		--env "LDHDNS_SUBDOMAIN_LABEL=$(SUBDOMAIN_LABEL)" \
		--security-opt "apparmor=unconfined" \
		--volume "/var/run/docker.sock:/tmp/docker.sock" \
		--volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
		--restart always \
		$(IMAGE):$(VERSION)

.PHONY: uninstall
uninstall:

	@docker stop ldhdns || true
	@docker stop ldhdns_dns || true
	@docker rm --force ldhdns || true
	@docker network rm ldhdns || true
