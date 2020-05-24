VERSION:=$(shell git symbolic-ref -q --short HEAD || git describe --tags --exact-match)

build:
	@echo ">> building binaries - darwin_amd64"
	@GOOS=darwin GOARCH=amd64 go build -ldflags="-X main.version=$(VERSION)" -o build/plt-darwin-amd64 plt.go && gzip -f build/plt-darwin-amd64
	@echo ">> building binaries - linux_amd64"
	@GOOS=linux GOARCH=amd64 go build -ldflags="-X main.version=$(VERSION)" -o build/plt-linux-amd64 plt.go && gzip -f build/plt-linux-amd64
	@echo ">> building binaries - windows_amd64"
	@GOOS=windows GOARCH=amd64 go build -ldflags="-X main.version=$(VERSION)" -o build/plt-windows-amd64.exe plt.go \
		&& zip -9 build/plt-windows-amd64.zip build/plt-windows-amd64.exe && rm build/plt-windows-amd64.exe
	@echo ">> building binaries - alpine_amd64"
	@docker run --rm -v $(PWD):/app -w /app golang:1.14-alpine go build -ldflags="-X main.version=$(VERSION)" -o build/plt-alpine-amd64 \
		&& gzip -f build/plt-alpine-amd64


.PHONY: build
