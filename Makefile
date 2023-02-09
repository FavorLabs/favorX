GO ?= go
GOBIND ?= gobind
GOMOBILE ?= gomobile
GOLANGCI_LINT ?= $$($(GO) env GOPATH)/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.46.2
GOGOPROTOBUF ?= protoc-gen-gogofaster
GOGOPROTOBUF_VERSION ?= v1.3.2

GO_MIN_VERSION ?= 1.18
GO_MOD_ENABLED_VERSION ?= 1.12
GO_MOD_VERSION ?= $(shell go mod edit -print | awk '/^go[ \t]+[0-9]+\.[0-9]+(\.[0-9]+)?[ \t]*$$/{print $$2}')
GO_SYSTEM_VERSION ?= $(shell go version | awk '{ gsub(/go/, "", $$3); print $$3 }')

COMMIT_HASH ?= $(shell git describe --long --dirty --always --match "" || true)
CLEAN_COMMIT ?= $(shell git describe --long --always --match "" || true)
COMMIT_TIME ?= $(shell git show -s --format=%ct $(CLEAN_COMMIT) || true)
LDFLAGS ?= -s -w -X github.com/FavorLabs/favorX.commitHash="$(COMMIT_HASH)" -X github.com/FavorLabs/favorX.commitTime="$(COMMIT_TIME)"

GOOS ?= $(shell go env GOOS)
SHELL ?= bash
IS_DOCKER ?= false
LIB_INSTALL_DIR ?= $(shell pwd)/thirdparty
CGO_ENABLED ?= $(shell go env CGO_ENABLED)

ifeq ($(GOOS), windows)
WORK_DIR := $(shell pwd | sed -E 's/^\/([a-zA-Z])\//\1\:\//' | sed -E 's/\//\\\\/g' | tr '[:upper:]' '[:lower:]')
PATH_SEP := ;
BINARY_NAME := favorX.exe
CGO_CFLAGS ?= -Ic:/wiredtiger/include
CGO_LDFLAGS ?= -Lc:/wiredtiger/lib
else
WORK_DIR := $(shell pwd | tr '[:upper:]' '[:lower:]')
PATH_SEP := :
BINARY_NAME := favorX
CGO_CFLAGS ?= -I$(LIB_INSTALL_DIR)/include
CGO_LDFLAGS ?= -L$(LIB_INSTALL_DIR)/lib
endif

.PHONY: all
all: check-version lint vet test-race binary

.PHONY: binary-ldb
binary-ldb: dist
binary-ldb:
	$(GO) env -w CGO_ENABLED=0
	$(GO) build -tags leveldb -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME) ./cmd/favorX
	$(GO) env -w CGO_ENABLED=$(CGO_ENABLED)

.PHONY: binary
binary: dist FORCE
	$(GO) version
ifneq ($(GOOS), windows)
	[ -d $(LIB_INSTALL_DIR) ] || mkdir $(LIB_INSTALL_DIR)
	sh -c "./install-deps.sh $(LIB_INSTALL_DIR) $(IS_DOCKER)"
endif
	$(GO) env -w CGO_ENABLED=1
	LD_LIBRARY_PATH=$(LIB_INSTALL_DIR) CGO_CFLAGS=$(CGO_CFLAGS) CGO_LDFLAGS=$(CGO_LDFLAGS) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY_NAME) ./cmd/favorX
	$(GO) env -w CGO_ENABLED=$(CGO_ENABLED)
ifeq ($(GOOS), darwin)
	$(MAKE) check-xcode
	for library in libwiredtiger libtcmalloc libsnappy; do \
		match_library_path=`otool -L dist/$(BINARY_NAME) | grep $$library | grep -oE '[^\s].*dylib'` ; \
		match_library=`echo $$match_library_path | grep -oE $$library'.*dylib'`; \
		install_name_tool -change $$match_library_path "@rpath/"$$match_library dist/${BINARY_NAME} ; \
	done
	install_name_tool -add_rpath @loader_path dist/${BINARY_NAME}
	install_name_tool -add_rpath @loader_path/../lib dist/${BINARY_NAME}
	install_name_tool -add_rpath @loader_path/../thirdparty/lib dist/${BINARY_NAME}
	wiredtiger_lib=`ls $(LIB_INSTALL_DIR)/lib/libwiredtiger-*.dylib | xargs realpath`; \
	tcmalloc_lib_path=`otool -L $$wiredtiger_lib | grep -w libtcmalloc | grep -oE '^.*dylib'`; \
	tcmalloc_lib=`echo $$tcmalloc_lib_path | grep -oE 'libtcmalloc[^/]*dylib'` ; \
	install_name_tool -change $$tcmalloc_lib_path "@rpath/"$$tcmalloc_lib $$wiredtiger_lib; \
	install_name_tool -change $$tcmalloc_lib_path "@rpath/"$$tcmalloc_lib $(LIB_INSTALL_DIR)/lib/libwiredtiger_snappy.so
else ifeq ($(GOOS), linux)
ifeq (, $(shell command -v patchelf 2>/dev/null))
	echo "patchelf not installed" && exit
endif
	patchelf --set-rpath '$$ORIGIN:$$ORIGIN/../lib:$$ORIGIN/../thirdparty/lib' dist/${BINARY_NAME}
	wiredtiger_lib=`ls $(LIB_INSTALL_DIR)/lib/libwiredtiger-*.so | xargs realpath`; \
	patchelf --set-rpath '$$ORIGIN' $$wiredtiger_lib
	wt_snappy_lib=`ls $(LIB_INSTALL_DIR)/lib/libwiredtiger_snappy.so | xargs realpath`; \
	patchelf --set-rpath '$$ORIGIN' $$wt_snappy_lib
endif

dist:
	mkdir $@
	$(GO) version

.PHONY: lint
lint: linter
	$(GOLANGCI_LINT) run

.PHONY: linter
linter:
	test -f $(GOLANGCI_LINT) || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$($(GO) env GOPATH)/bin $(GOLANGCI_LINT_VERSION)

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test-race
test-race:
ifdef cover
	$(GO) test -race -failfast -coverprofile=cover.out -v ./...
else
	$(GO) test -race -failfast -v ./...
endif

.PHONY: test-integration
test-integration:
	$(GO) test -tags=integration -v ./...

.PHONY: test
test:
	$(GO) test -v -failfast ./...

.PHONY: build
build: export CGO_ENABLED=0
build: check-version
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" ./...

.PHONY: android
android: check-java
android: check-mobile-tool
android: download-vendor
android:
	[ -d "dist" ] || mkdir dist
	[ -f "dist/favorX.aar" ] || ($(GOMOBILE) bind -androidapi 26 -tags=leveldb -target=android -o=favorX.aar -ldflags="$(LDFLAGS)" ./mobile && mv -n favorX.aar dist/ && mv -n favorX-sources.jar dist/) || (echo "build android sdk failed" && exit 1)
	rm -rf vendor
	echo "android sdk build finished."
	echo "please import dist/favorX.aar to android studio!"

.PHONY: ios
ios: check-xcode
ios: check-mobile-tool
ios: download-vendor
ios:
	[ -d "dist" ] || mkdir dist
	[ -d "dist/favorX.xcframework" ] || ($(GOMOBILE) bind -tags=leveldb,nowatchdog -target=ios -o=favorX.xcframework -ldflags="$(LDFLAGS)" ./mobile && mv -n favorX.xcframework dist/) || (echo "build ios framework failed" && exit 1)
	rm -rf vendor
	echo "ios framework build finished."
	echo "please import dist/favorX.xcframework to xcode!"

.PHONY: check-mobile-tool
check-mobile-tool: check-version
check-mobile-tool:
	check_path=false; for line in $(shell $(GO) env GOPATH | tr $(PATH_SEP) '\n' | tr '[:upper:]' '[:lower:]'); do if [[ $(WORK_DIR) =~ ^$$line ]]; then check_path=true; fi; done; $$check_path || (echo "Current path does not match your GOPATH, please check" && exit 1)
	type ${GOMOBILE} || $(GO) install golang.org/x/mobile/cmd/gomobile@latest
	${GOMOBILE} init
	type ${GOBIND} || $(GO) install golang.org/x/mobile/cmd/gobind@latest
	$(GO) get golang.org/x/mobile/bind

.PHONY: check-java
check-java:
	type java || (echo "Not found java on the system" && exit 1)
	java -version || (echo "Java check version failed, please check java setup" && exit 1)
	[ -z $(ANDROID_HOME) ] && echo "Please set ANDROID_HOME env" && exit 1; exit 0
	[ -z $(ANDROID_NDK_HOME) ] && echo "Please install android NDK tools, and set ANDROID_NDK_HOME" && exit 1; exit 0

.PHONY: check-xcode
check-xcode:
	[ ${GOOS} = "darwin" ] || (echo "Must be on the MacOS system" && exit 1)
	xcode-select -p || (echo "Please install command line tool first" && exit 1)
	xcrun xcodebuild -version || (echo "Please install Xcode. If xcode installed, you should exec `sudo xcode-select -s /Applications/Xcode.app/Contents/Developer` in your shell" && exit 1)

.PHONY: download-vendor
download-vendor:
	$(GO) mod download

.PHONY: check-version
check-version:
	[ ${GO_SYSTEM_VERSION} \< ${GO_MOD_ENABLED_VERSION} ] && echo "The version of Golang on the system (${GO_SYSTEM_VERSION}) is too old and does not support go modules. Please use at least ${GO_MIN_VERSION}." && exit 1; exit 0
	[ ${GO_SYSTEM_VERSION} \< ${GO_MIN_VERSION} ] && echo "The version of Golang on the system (${GO_SYSTEM_VERSION}) is below the minimum required version (${GO_MIN_VERSION}) and therefore will not build correctly." && exit 1; exit 0

.PHONY: githooks
githooks:
	ln -f -s ../../.githooks/pre-push.bash .git/hooks/pre-push

.PHONY: protobuftools
protobuftools:
	which protoc || ( echo "install protoc for your system from https://github.com/protocolbuffers/protobuf/releases" && exit 1)
	which $(GOGOPROTOBUF) || ( cd /tmp && GO111MODULE=on $(GO) get -u github.com/gogo/protobuf/$(GOGOPROTOBUF)@$(GOGOPROTOBUF_VERSION) )

.PHONY: protobuf
protobuf: GOFLAGS=-mod=mod # use modules for protobuf file include option
protobuf: protobuftools
	$(GO) generate -run protoc ./...

.PHONY: clean
clean:
	$(GO) clean
	rm -rf dist/

FORCE:
