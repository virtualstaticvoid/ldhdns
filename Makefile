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
		--build-arg VERSION=$(VERSION) \
		--build-arg S6_VERSION=$(S6_VERSION) \
		--tag $(IMAGE_NAME):latest \
		--tag $(IMAGE_NAME):$(VERSION) \
		.

.PHONY: run
run: build

	docker compose up --force-recreate --build || true

.PHONY: test
test:

	docker compose run -it --rm test

.PHONY: clean
clean:

	docker compose down --volumes

# adapted from https://stackoverflow.com/a/48782113/30521
# used by the `Load Makefile.vars` build step of GitHub Actions
env-%:
	@echo '$($*)'
