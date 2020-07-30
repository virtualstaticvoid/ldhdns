# variables
include Makevars

export

# default build target
all::

.PHONY: all
all:: build

.PHONY: build
build:

	docker build \
		--tag $(IMAGE):latest \
		--tag $(IMAGE):$(VERSION) \
		--tag $(DOCKER_REPO)/$(IMAGE):latest \
		--tag $(DOCKER_REPO)/$(IMAGE):$(VERSION) \
		--build-arg VERSION=$(VERSION) \
		--label "git.sha=$(GIT_SHA)" \
		--label "git.date=$(GIT_DATE)" \
		--label "build.date=$(BUILD_DATE)" \
		--label "maintainer=$(MAINTAINER)" \
		--label "maintainer.url=$(MAINTAINER_URL)" \
		--label "build.logurl=$(TRAVIS_BUILD_WEB_URL)" \
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
