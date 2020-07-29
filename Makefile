# variables
IMAGE:=ldhdns
DOCKER_REPO:=virtualstaticvoid
VERSION?=latest

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

	docker run \
		--rm \
		--interactive \
		--tty \
		--network host \
		--security-opt "apparmor=unconfined" \
		--volume "/var/run/docker.sock:/tmp/docker.sock" \
		--volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
		$(IMAGE):$(VERSION)

.PHONY: run
run:

	# TODO: check if container exists already
	docker run \
		--name ldhdns \
		--detach \
		--network host \
		--restart always \
		--security-opt "apparmor=unconfined" \
		--volume "/var/run/docker.sock:/tmp/docker.sock" \
		--volume "/var/run/dbus/system_bus_socket:/var/run/dbus/system_bus_socket" \
		$(IMAGE):$(VERSION)

.PHONY: clean
clean:

	@docker stop ldhdns || true
	@docker stop ldhdns_dns || true
	@docker rm --force ldhdns || true
	@docker network rm ldhdns || true
