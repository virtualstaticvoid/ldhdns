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
		--label org.opencontainers.image.title="ldhdns" \
		--label org.opencontainers.image.description="A developer tool for providing DNS for Docker containers running on a local development host." \
		--label org.opencontainers.image.source="$(MAINTAINER_URL)" \
		--label org.opencontainers.image.authors="$(MAINTAINER)" \
		--label org.opencontainers.image.url="$(MAINTAINER_URL)" \
		--label org.opencontainers.image.created="$(BUILD_DATE)" \
		--label org.opencontainers.image.version="$(VERSION)" \
		--label org.opencontainers.image.revision="$(GIT_SHA)" \
		--label build.logurl="$(TRAVIS_BUILD_WEB_URL)" \
		.

.PHONY: debug
debug:

	@# write env vars
	@echo "NETWORK_ID=$(NETWORK_ID)" 						 > .env
	@echo "DOMAIN_SUFFIX=$(DOMAIN_SUFFIX)" 			>> .env
	@echo "SUBDOMAIN_LABEL=$(SUBDOMAIN_LABEL)" 	>> .env

	docker-compose up --detach

.PHONY: publish
publish:

	docker push $(DOCKER_REPO)/$(IMAGE):latest
	docker push $(DOCKER_REPO)/$(IMAGE):$(VERSION)

.PHONY: install
install: build

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
		--restart unless-stopped \
		$(IMAGE):$(VERSION)

.PHONY: uninstall
uninstall:

	@docker stop ldhdns || true
	@docker stop ldhdns_dns || true
	@docker rm --force ldhdns || true
	@docker network rm ldhdns || true
