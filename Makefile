VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT  := $(shell git rev-parse --short HEAD)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: build test lint dist clean

build:
	@go build -ldflags "$(LDFLAGS)" -o filetap .

test:
	@go test ./...

lint:
	@go vet ./...

dist:
	@rm -rf dist
	@mkdir -p dist
	@cp LICENSE README.md dist/
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		bin=filetap; \
		if [ "$$os" = "windows" ]; then bin=filetap.exe; fi; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o dist/$$bin . && \
		(cd dist && zip -q filetap-$(VERSION)-$$os-$$arch.zip $$bin LICENSE README.md && rm $$bin); \
	done
	@rm dist/LICENSE dist/README.md
	@cd dist && shasum -a 256 *.zip > checksums.txt

clean:
	@rm -f filetap
	@rm -rf dist
