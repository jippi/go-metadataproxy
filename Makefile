VETARGS		?=-all
GIT_COMMIT 	:= $(shell git describe --tags)
GIT_DIRTY 	:= $(if $(shell git status --porcelain),+CHANGES)
GO_LDFLAGS 	:= "-X main.GitCommit=$(GIT_COMMIT)$(GIT_DIRTY)"
BUILD_DIR ?= $(abspath build)

.PHONY: $(BUILD_DIR)
$(BUILD_DIR):
	@mkdir -p $@

GOBUILD ?= $(shell go env GOOS)-$(shell go env GOARCH)

GET_GOOS   = $(word 1,$(subst -, ,$1))
GET_GOARCH = $(word 2,$(subst -, ,$1))

.PHONY: go-mod-download
go-mod-download:
	@echo "=> go mod download"
	@go mod download

BINARIES = $(addprefix $(BUILD_DIR)/go-metadataproxy-, $(GOBUILD))
$(BINARIES): $(BUILD_DIR)/go-metadataproxy-%: $(BUILD_DIR) go-mod-download
	@echo "=> building $@ ..."
	GOOS=$(call GET_GOOS,$*) GOARCH=$(call GET_GOARCH,$*) CGO_ENABLED=0 go build -o $@ -ldflags $(GO_LDFLAGS)

.PHONY: build
build:
	@echo "=> building binaries ..."
	$(MAKE) -j $(BINARIES)

.PHONY: rebuild
rebuild: clean
	@echo "=> rebuilding binaries ..."
	$(MAKE) -j build

.PHONY: clean
clean:
	@echo "=> cleaning binaries ..."
	rm -rf $(BUILD_DIR)

.PHONY: test
test:
	@echo "==> Running $@..."
	@go test -v -tags $(shell go list ./... | grep -v vendor)

.PHONY: docker
docker:
	@echo "=> build and push Docker image ..."
	docker build -f Dockerfile -t jippi/go-metadataproxy:$(COMMIT) .
	docker tag jippi/go-metadataproxy:$(COMMIT) jippi/go-metadataproxy:$(TAG)
	docker push jippi/go-metadataproxy:$(TAG)
