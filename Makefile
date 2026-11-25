VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -s -w -X github.com/aic0d3r/z13ctl-plus/cmd.Version=$(VERSION)

SYSTEMD_USER_DIR  := $(HOME)/.config/systemd/user
SYSTEMD_SYSTEM_DIR := /etc/systemd/system

.PHONY: build test cover lint mod-tidy snapshot release install install-service uninstall-service install-perms-service uninstall-perms-service docs clean help

## build: compile z13ctl-plus with version from git tags
build:
	go build -ldflags "$(LDFLAGS)" -o z13ctl-plus .

## test: run all tests
test:
	go test ./...

## cover: run tests with coverage report
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## mod-tidy: tidy go.mod for all modules in the repo
mod-tidy:
	go mod tidy
	cd api && go mod tidy

## snapshot: build a local snapshot release via goreleaser (no publish)
snapshot:
	goreleaser release --snapshot --clean

## release: publish a release via goreleaser (requires a clean git tag)
release:
	goreleaser release --clean

## install: install z13ctl-plus binary to /usr/local/bin (requires sudo, build first)
install:
	install -Dm755 z13ctl-plus /usr/local/bin/z13ctl-plus

## install-service: install and enable the z13ctl-plus systemd user service
install-service:
	install -Dm644 contrib/systemd/user/z13ctl-plus.socket $(SYSTEMD_USER_DIR)/z13ctl-plus.socket
	install -Dm644 contrib/systemd/user/z13ctl-plus.service $(SYSTEMD_USER_DIR)/z13ctl-plus.service
	systemctl --user daemon-reload
	systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service
	@echo "Service installed. Run 'systemctl --user status z13ctl-plus.service' to verify."

## uninstall-service: stop and remove the z13ctl-plus systemd user service
uninstall-service:
	-systemctl --user disable --now z13ctl-plus.socket z13ctl-plus.service
	rm -f $(SYSTEMD_USER_DIR)/z13ctl-plus.socket $(SYSTEMD_USER_DIR)/z13ctl-plus.service
	systemctl --user daemon-reload
	@echo "Service removed."

## install-perms-service: install the boot permissions service (requires sudo and an installed z13ctl-plus)
# setup writes the unit with the installed binary's path (/usr/local/bin for make install).
install-perms-service:
	z13ctl-plus setup
	systemctl restart z13ctl-plus-perms.service
	@echo "Permissions service installed. Run 'systemctl status z13ctl-plus-perms' to verify."

## uninstall-perms-service: remove the sysfs permissions service (requires sudo)
uninstall-perms-service:
	-systemctl disable --now z13ctl-plus-perms.service
	rm -f $(SYSTEMD_SYSTEM_DIR)/z13ctl-plus-perms.service
	systemctl daemon-reload
	@echo "Permissions service removed."

## docs: serve mkdocs locally
docs:
	mkdocs serve

## clean: remove all generated build and test artifacts
clean:
	rm -f z13ctl-plus
	rm -rf dist/
	find . -name '*.test' -delete
	find . -name 'coverage.out' -o -name 'coverage.*' -o -name '*.coverprofile' -o -name 'profile.cov' | xargs rm -f

## help: list available targets
help:
	@grep -E '^##' Makefile | sed 's/^## /  /'
