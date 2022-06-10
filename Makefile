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
		--build-arg S6_VERSION=$(S6_VERSION) \
		--label org.opencontainers.image.title="ldhdns" \
		--label org.opencontainers.image.description="A developer tool for providing DNS for Docker containers running on a local development host." \
		--label org.opencontainers.image.source="$(MAINTAINER_URL)" \
		--label org.opencontainers.image.authors="$(MAINTAINER)" \
		--label org.opencontainers.image.url="$(MAINTAINER_URL)" \
		--label org.opencontainers.image.created="$(BUILD_DATE)" \
		--label org.opencontainers.image.version="$(VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_SHA)" \
		.

.PHONY: debug
debug:

	docker-compose up

.PHONY: publish
publish:

	docker push $(DOCKER_REPO)/$(IMAGE):latest
	docker push $(DOCKER_REPO)/$(IMAGE):$(VERSION)

.PHONY: install
install: build

	docker run \
		--name $(LDHDNS_CONTAINER_NAME) \
		--detach \
		--network host \
		--env LDHDNS_NETWORK_ID=$(LDHDNS_NETWORK_ID) \
		--env LDHDNS_DOMAIN_SUFFIX=$(LDHDNS_DOMAIN_SUFFIX) \
		--env LDHDNS_SUBDOMAIN_LABEL=$(LDHDNS_SUBDOMAIN_LABEL) \
		--env LDHDNS_CONTAINER_NAME=$(LDHDNS_CONTAINER_NAME) \
		--security-opt "apparmor=unconfined" \
		--volume "/var/run/docker.sock:/tmp/docker.sock" \
		--volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
		--restart unless-stopped \
		$(IMAGE):$(VERSION)

.PHONY: uninstall
uninstall:

	@docker stop $(LDHDNS_CONTAINER_NAME)       || true
	@docker stop $(LDHDNS_CONTAINER_NAME)_dns   || true
	@docker rm --force $(LDHDNS_CONTAINER_NAME) || true
	@docker network rm $(LDHDNS_CONTAINER_NAME) || true

# adapted from https://stackoverflow.com/a/48782113/30521
# used by the `Load Makefile.vars` build step of GitHub Actions
env-%:
	@echo '$($*)'
