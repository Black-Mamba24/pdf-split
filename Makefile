BINDIR ?= $(HOME)/.local/bin
GOHOSTARCH := $(shell go env GOHOSTARCH)

.PHONY: build install uninstall

build:
	GOARCH=$(GOHOSTARCH) CGO_ENABLED=0 go build -o pdf-split ./cmd/pdf-split

install:
	mkdir -p "$(BINDIR)"
	GOBIN="$(BINDIR)" GOARCH=$(GOHOSTARCH) CGO_ENABLED=0 go install ./cmd/pdf-split
	@echo "Installed pdf-split to $(BINDIR)/pdf-split"

uninstall:
	rm -f "$(BINDIR)/pdf-split"
