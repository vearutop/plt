#GOLANGCI_LINT_VERSION := "v1.38.0" # Optional configuration to pinpoint golangci-lint version.

# The head of Makefile determines location of dev-go to include standard targets.
GO ?= go
export GO111MODULE = on

ifneq "$(GOFLAGS)" ""
  $(info GOFLAGS: ${GOFLAGS})
endif

ifneq "$(wildcard ./vendor )" ""
  $(info Using vendor)
  modVendor =  -mod=vendor
  ifeq (,$(findstring -mod,$(GOFLAGS)))
      export GOFLAGS := ${GOFLAGS} ${modVendor}
  endif
  ifneq "$(wildcard ./vendor/github.com/bool64/dev)" ""
  	DEVGO_PATH := ./vendor/github.com/bool64/dev
  endif
endif

ifeq ($(DEVGO_PATH),)
	DEVGO_PATH := $(shell GO111MODULE=on $(GO) list ${modVendor} -f '{{.Dir}}' -m github.com/bool64/dev)
	ifeq ($(DEVGO_PATH),)
    	$(info Module github.com/bool64/dev not found, downloading.)
    	DEVGO_PATH := $(shell export GO111MODULE=on && $(GO) mod tidy && $(GO) list -f '{{.Dir}}' -m github.com/bool64/dev)
	endif
endif

-include $(DEVGO_PATH)/makefiles/main.mk
-include $(DEVGO_PATH)/makefiles/lint.mk
-include $(DEVGO_PATH)/makefiles/test-unit.mk
-include $(DEVGO_PATH)/makefiles/reset-ci.mk

build:
	@echo ">> building binaries - darwin_amd64"
	@GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt-darwin-amd64 plt.go && gzip -f build/plt-darwin-amd64
	@echo ">> building binaries - linux_amd64"
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt-linux-amd64 plt.go && gzip -f build/plt-linux-amd64
	@echo ">> building binaries - windows_amd64"
	@GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt-windows-amd64.exe plt.go \
		&& zip -9 -j build/plt-windows-amd64.zip build/plt-windows-amd64.exe && rm build/plt-windows-amd64.exe
	@echo ">> building binaries - alpine_amd64"
	@docker run --rm -v $(PWD):/app -w /app golang:1.15-alpine go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt-alpine-amd64 \
		&& gzip -f build/plt-alpine-amd64

.PHONY: build