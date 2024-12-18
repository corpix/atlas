.DEFAULT_GOAL := all

version = $(shell date +"%Y-%m-%d").$(shell git rev-list --count HEAD)

goverter = goverter gen \
	-output-constraint '' \
	-g 'wrapErrors yes' \
	-g 'useZeroValueOnPointerInconsistency yes' \
	-g 'ignoreMissing no' \
	-g 'skipCopySameType yes' \
	-g 'ignoreUnexported yes' \
	-g 'matchIgnoreCase yes' \
	-g 'enum no'

.PHONY: all
all: test

.PHONY: lint
lint:
	go vet ./...
	golangci-lint run -v

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: test
test: lint
	go test -v ./...

.PHONY: tag
tag:
	git tag v$(version)
