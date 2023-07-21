# The binary to build (just the basename).
# This can be overwritten from command line using
BIN := workerpodautoscaler

UNIQUE:=$(shell date +%s)

# Where to push the docker image.
REGISTRY ?= public.ecr.aws/practo

BASE_BRANCH := master
CURRENT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
# This version-strategy uses git tags to set the version string
VERSION := $(shell git describe --tags --always --dirty)
MAJOR_VERSION = $(shell git describe --tags  --dirty | \
	awk -F'.' '{print $$1}')
MAJOR_MINOR_VERSION = $(shell git describe --tags  --dirty | \
	awk -F'.' '{print $$1"."$$2}')
#
# This version-strategy uses a manual value to set the version string
#VERSION := 1.2.3

###
### These variables should not need tweaking.
###

# directories which hold app source (not vendored)
SRC_DIRS := cmd pkg/cmdutil pkg/controller pkg/queue pkg/signals pkg/version

ALL_PLATFORMS := darwin/amd64 linux/amd64
# linux/arm linux/arm64 linux/ppc64le linux/s390x

# Used internally.  Users should pass GOOS and/or GOARCH.
OS := $(if $(GOOS),$(GOOS),$(shell go env GOOS))
ARCH := $(if $(GOARCH),$(GOARCH),$(shell go env GOARCH))

BASEIMAGE ?= gcr.io/distroless/static

IMAGE := $(REGISTRY)/$(BIN)

TAG := $(VERSION)
MAJOR_TAG := $(MAJOR_VERSION)
MAJOR_MINOR_TAG := $(MAJOR_MINOR_VERSION)

PUBLISH_TAGS := $(IMAGE):$(TAG) $(IMAGE):$(MAJOR_MINOR_VERSION) $(IMAGE):$(MAJOR_VERSION)
ifneq (,$(findstring -beta,$(TAG)))
    PUBLISH_TAGS := $(IMAGE):$(TAG) $(IMAGE):$(MAJOR_MINOR_VERSION)-beta $(IMAGE):$(MAJOR_VERSION)-beta
endif
ifneq (,$(findstring -alpha,$(TAG)))
    PUBLISH_TAGS := $(IMAGE):$(TAG) $(IMAGE):$(MAJOR_MINOR_VERSION)-alpha $(IMAGE):$(MAJOR_VERSION)-alpha
endif
ifneq ($(CURRENT_BRANCH), $(BASE_BRANCH))
	PUBLISH_TAGS := $(IMAGE):$(TAG)
endif
ifneq (,$(findstring -dirty,$(TAG)))
	PUBLISH_TAGS := $(IMAGE):$(TAG)
endif

define \n


endef

BUILD_IMAGE ?= golang:1.17.1-alpine
TEST_IMAGE ?= public.ecr.aws/practo/golang:1.17.1-alpine-test

# If you want to build all binaries, see the 'all-build' rule.
# If you want to build all containers, see the 'all-container' rule.
# If you want to build AND push all containers, see the 'all-push' rule.
all: build

# For the following OS/ARCH expansions, we transform OS/ARCH into OS_ARCH
# because make pattern rules don't match with embedded '/' characters.

build-%:
	@$(MAKE) build                        \
	    --no-print-directory              \
	    GOOS=$(firstword $(subst _, ,$*)) \
	    GOARCH=$(lastword $(subst _, ,$*))

container-%:
	@$(MAKE) container                    \
	    --no-print-directory              \
	    GOOS=$(firstword $(subst _, ,$*)) \
	    GOARCH=$(lastword $(subst _, ,$*))

push-%:
	@$(MAKE) push                         \
	    --no-print-directory              \
	    GOOS=$(firstword $(subst _, ,$*)) \
	    GOARCH=$(lastword $(subst _, ,$*))

all-build: $(addprefix build-, $(subst /,_, $(ALL_PLATFORMS)))

all-container: $(addprefix container-, $(subst /,_, $(ALL_PLATFORMS)))

all-push: $(addprefix push-, $(subst /,_, $(ALL_PLATFORMS)))

fmt:
	@echo "formatting code"
	/bin/sh -c "./hack/format.sh"

build: bin/$(OS)_$(ARCH)/$(BIN)

# Directories that we need created to build/test.
BUILD_DIRS := bin/$(OS)_$(ARCH)     \
              .go/bin/$(OS)_$(ARCH) \
              .go/cache

# The following structure defeats Go's (intentional) behavior to always touch
# result files, even if they have not changed.  This will still run `go` but
# will not trigger further work if nothing has actually changed.
OUTBIN = bin/$(OS)_$(ARCH)/$(BIN)
$(OUTBIN): .go/$(OUTBIN).stamp
	@true

# This will build the binary under ./.go and update the real binary iff needed.
.PHONY: .go/$(OUTBIN).stamp
.go/$(OUTBIN).stamp: $(BUILD_DIRS)
	@echo "making $(OUTBIN)"
	@docker run                                                 \
	    -i                                                      \
	    --rm                                                    \
	    -u $$(id -u):$$(id -g)                                  \
	    -v $$(pwd):/src                                         \
	    -w /src                                                 \
	    -v $$(pwd)/.go/bin/$(OS)_$(ARCH):/go/bin                \
	    -v $$(pwd)/.go/bin/$(OS)_$(ARCH):/go/bin/$(OS)_$(ARCH)  \
	    -v $$(pwd)/.go/cache:/.cache                            \
	    --env HTTP_PROXY=$(HTTP_PROXY)                          \
	    --env HTTPS_PROXY=$(HTTPS_PROXY)                        \
	    $(BUILD_IMAGE)                                          \
	    /bin/sh -c "                                            \
	        ARCH=$(ARCH)                                        \
	        OS=$(OS)                                            \
	        VERSION=$(VERSION)                                  \
	        ./hack/build.sh                                    \
	    "
	@if ! cmp -s .go/$(OUTBIN) $(OUTBIN); then \
	    mv .go/$(OUTBIN) $(OUTBIN);            \
	    date >$@;                              \
	fi

# Example: make shell CMD="-c 'date > datefile'"
shell: $(BUILD_DIRS)
	@echo "launching a shell in the containerized build environment"
	@docker run                                                 \
	    -ti                                                     \
	    --rm                                                    \
	    -u $$(id -u):$$(id -g)                                  \
	    -v $$(pwd):/src                                         \
	    -w /src                                                 \
	    -v $$(pwd)/.go/bin/$(OS)_$(ARCH):/go/bin                \
	    -v $$(pwd)/.go/bin/$(OS)_$(ARCH):/go/bin/$(OS)_$(ARCH)  \
	    -v $$(pwd)/.go/cache:/.cache                            \
	    --env HTTP_PROXY=$(HTTP_PROXY)                          \
	    --env HTTPS_PROXY=$(HTTPS_PROXY)                        \
	    $(BUILD_IMAGE)                                          \
	    /bin/sh $(CMD)

# Used to track state in hidden files.
DOTFILE_IMAGE = $(subst /,_,$(IMAGE))-$(TAG)

container: .container-$(DOTFILE_IMAGE) say_container_name
.container-$(DOTFILE_IMAGE): bin/$(OS)_$(ARCH)/$(BIN) Dockerfile.in
	@sed                                 \
	    -e 's|{ARG_BIN}|$(BIN)|g'        \
	    -e 's|{ARG_ARCH}|$(ARCH)|g'      \
	    -e 's|{ARG_OS}|$(OS)|g'          \
	    -e 's|{ARG_FROM}|$(BASEIMAGE)|g' \
	    Dockerfile.in > .dockerfile-$(OS)_$(ARCH)
	@docker build $(foreach T, $(PUBLISH_TAGS), -t $(T)) -f .dockerfile-$(OS)_$(ARCH) .
	@docker images -q $(IMAGE):$(TAG) > $@

say_container_name:
	@echo "container: $(IMAGE):$(TAG)"

push: .push-$(DOTFILE_IMAGE) say_push_name
.push-$(DOTFILE_IMAGE): .container-$(DOTFILE_IMAGE)
	@$(foreach T, $(PUBLISH_TAGS), docker push $(T) $(\n))

say_push_name:
	@$(foreach T, $(PUBLISH_TAGS), echo "pushed: $(T)" $(\n))

manifest-list: push
	platforms=$$(echo $(ALL_PLATFORMS) | sed 's/ /,/g');  \
	manifest-tool                                         \
	    --username=oauth2accesstoken                      \
	    --password=$$(gcloud auth print-access-token)     \
	    push from-args                                    \
	    --platforms "$$platforms"                         \
	    --template $(REGISTRY)/$(BIN):$(VERSION)__OS_ARCH \
	    --target $(REGISTRY)/$(BIN):$(VERSION)

version:
	@echo $(VERSION)

generate:
	./hack/update-codegen.sh

test: $(BUILD_DIRS)
	@docker run                                                 \
	    -i                                                      \
	    --rm                                                    \
	    -u $$(id -u):$$(id -g)                                  \
	    -v $$(pwd):/src                                         \
	    -w /src                                                 \
	    -v $$(pwd)/.go/bin/$(OS)_$(ARCH):/go/bin                \
	    -v $$(pwd)/.go/bin/$(OS)_$(ARCH):/go/bin/$(OS)_$(ARCH)  \
	    -v $$(pwd)/.go/cache:/.cache                            \
	    --env HTTP_PROXY=$(HTTP_PROXY)                          \
	    --env HTTPS_PROXY=$(HTTPS_PROXY)                        \
	    $(TEST_IMAGE)                                          \
	    /bin/sh -c "                                            \
	        ARCH=$(ARCH)                                        \
	        OS=$(OS)                                            \
	        VERSION=$(VERSION)                                  \
	        ./hack/test.sh $(SRC_DIRS)                         \
	    "

$(BUILD_DIRS):
	@mkdir -p $@

clean: container-clean bin-clean

container-clean:
	rm -rf .container-* .dockerfile-* .push-*

bin-clean:
	rm -rf .go bin
