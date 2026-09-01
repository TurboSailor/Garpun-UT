# Pulse for Ubuntu Touch.
#
# The click tree is assembled in build/pkg, then packed *on the phone* (macOS has
# no `click` tool) and installed from there. See scripts/build.sh, scripts/deploy.sh.

APP     := cc.zachy.pulse
ARCH    := arm64
VERSION := $(shell sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' click/manifest.json)

PKG   := build/pkg
CLICK := build/$(APP)_$(VERSION)_$(ARCH).click

# Overridable so the packaging chain can be exercised against stub sources.
BACKEND_DIR ?= backend
QML_DIR     ?= qml

GO         := go
GO_BUILD   := CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) $(GO) build -trimpath -ldflags "-s -w"
CLICK_META := click/manifest.json click/pulse.apparmor click/pulse.desktop click/pulse.png

.PHONY: all backend qml meta pkg click deploy logs clean

all: click

# Cross-compiled daemon, Waydroid notification relay and diagnostic CLI.
backend:
	mkdir -p $(PKG)/bin
	cd $(BACKEND_DIR) && $(GO_BUILD) -o $(CURDIR)/$(PKG)/bin/pulsed ./cmd/pulsed
	cd $(BACKEND_DIR) && $(GO_BUILD) -o $(CURDIR)/$(PKG)/bin/pulse-wdnotify ./cmd/pulse-wdnotify
	cd $(BACKEND_DIR) && $(GO_BUILD) -o $(CURDIR)/$(PKG)/bin/pulsectl ./cmd/pulsectl

qml:
	rm -rf $(PKG)/qml
	mkdir -p $(PKG)
	cp -R $(QML_DIR) $(PKG)/qml

meta:
	mkdir -p $(PKG)
	cp $(CLICK_META) $(PKG)/
	cp click/run.sh $(PKG)/run.sh
	chmod +x $(PKG)/run.sh

pkg: backend qml meta

click: pkg
	scripts/build.sh $(PKG) $(CLICK)

deploy: click
	scripts/deploy.sh $(CLICK)

logs:
	scripts/logs.sh

clean:
	rm -rf build
