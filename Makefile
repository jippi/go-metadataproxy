VETARGS		?=-all
GIT_COMMIT 	:= $(shell git describe --tags)
GIT_DIRTY 	:= $(if $(shell git status --porcelain),+CHANGES)
GO_LDFLAGS 	:= "-X main.GitCommit=$(GIT_COMMIT)$(GIT_DIRTY)"
GOFILES_NOVENDOR = $(shell find . -type f -name '*.go' -not -path "./vendor/")
BUILD_DIR ?= $(abspath build)

$(BUILD_DIR):
	mkdir -p $@

GOBUILD ?= $(shell go env GOOS)-$(shell go env GOARCH)

GET_GOOS   = $(word 1,$(subst -, ,$1))
GET_GOARCH = $(word 2,$(subst -, ,$1))

BINARIES = $(addprefix $(BUILD_DIR)/go-metadaproxy-, $(GOBUILD))
$(BINARIES): $(BUILD_DIR)/go-metadaproxy-%: $(BUILD_DIR)
	@echo "=> building $@ ..."
	GOOS=$(call GET_GOOS,$*) GOARCH=$(call GET_GOARCH,$*) CGO_ENABLED=0 go build -o $@ -ldflags $(GO_LDFLAGS)

.PHONY: install
install:
	@echo "=> Installing dep"
	@go get -u github.com/golang/dep/cmd/dep

	@echo "=> dep ensure"
	@dep ensure

.PHONY: fmt
fmt:
	@echo "=> Running go fmt" ;
	@if [ -n "`go fmt . internal/...`" ]; then \
		echo "[ERR] go fmt updated formatting. Please commit formatted code first."; \
		exit 1; \
	fi

.PHONY: vet
vet: fmt
	@go tool vet 2>/dev/null ; if [ $$? -eq 3 ]; then \
		go get golang.org/x/tools/cmd/vet; \
	fi

	@echo "=> Running go tool vet $(VETARGS) ${GOFILES_NOVENDOR}"
	@go tool vet $(VETARGS) ${GOFILES_NOVENDOR} ; if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "[LINT] Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
	fi

.PHONY: build
build: install fmt vet
	@echo "=> building backend ..."
	$(MAKE) -j $(BINARIES)

.PHONY: rebuild
rebuild: clean
	@echo "=> rebuilding backend ..."
	$(MAKE) -j build

.PHONY: clean
clean:
	@echo "=> cleaning backend ..."
	rm -rf $(BUILD_DIR)

.PHONY: dist-clean
dist-clean: clean
	@echo "=> dist-cleaning backend ..."
	rm -rf vendor/

.PHONY: test
test:
	@echo "==> Running $@..."
	@go test -v -tags $(shell go list ./... | grep -v vendor)

.PHONY: docker
docker:
	@echo "=> build and push Docker image ..."
	@docker login -u $(DOCKER_USER) -p $(DOCKER_PASS)
	docker build -f Dockerfile -t jippi/go-metadataproxy:$(COMMIT) .
	docker tag jippi/go-metadataproxy:$(COMMIT) jippi/go-metadataproxy:$(TAG)
	docker push jippi/go-metadataproxy:$(TAG)

.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=amd64 go build -o build/go-metadaproxy-linux-amd64
	ssh 10.30.68.202 "sudo killall -9 go-metadaproxy-linux-amd64 || exit 0"
	scp build/go-metadaproxy-linux-amd64 10.30.68.202:/tmp
	ssh 10.30.68.202 "sudo COPY_DOCKER_ENV=PROJECT_VERSION COPY_DOCKER_LABELS=PROJECT_NAME /tmp/go-metadaproxy-linux-amd64"
