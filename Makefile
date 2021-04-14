export CGO_ENABLED = 0

#GOLANGCI_LINT_VERSION := "v1.38.0" # Optional configuration to pinpoint golangci-lint version.
VERSION := $(shell git symbolic-ref -q --short HEAD || git describe --tags --exact-match)

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
	@GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt plt.go && cd build && tar zcvf plt-darwin-amd64.tar.gz plt && rm plt
	@echo ">> building binaries - darwin_arm64"
	@GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt plt.go && cd build && tar zcvf plt-darwin-arm64.tar.gz plt && rm plt
	@echo ">> building binaries - linux_amd64"
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt plt.go && cd build && tar zcvf plt-linux-amd64.tar.gz plt && rm plt
	@echo ">> building binaries - linux_arm64"
	@GOOS=linux GOARCH=arm64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt plt.go && cd build && tar zcvf plt-linux-arm64.tar.gz plt && rm plt
	@echo ">> building binaries - linux_arm"
	@GOOS=linux GOARCH=arm go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt plt.go && cd build && tar zcvf plt-linux-arm.tar.gz plt && rm plt
	@echo ">> building binaries - windows_amd64"
	@GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -X main.version=$(VERSION)" -o build/plt.exe plt.go \
		&& zip -9 -j build/plt-windows-amd64.zip build/plt.exe && rm build/plt.exe

.PHONY: build