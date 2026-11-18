.PHONY: build build-all test e2e lint fmt snapshot

# the generic build, into bin/agent-cli
build:
	sh tools/build agent-cli

# every variant for this machine, into bin/
build-all:
	. tools/variants && for variant in $$VARIANTS; do sh tools/build $$variant || exit 1; done

test:
	go test -race -count=1 ./...

# every variant built with tools/build, then the e2e tests run against the binaries
e2e:
	sh scripts/e2e.sh

lint:
	golangci-lint run
	sh scripts/check-eof.sh

fmt:
	golangci-lint fmt

# release archives for every platform into dist/, without publishing
snapshot:
	sh tools/release ga --snapshot
