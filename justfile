set shell := ["bash", "-cu"]
set lazy := true

import? ".env.just"

hq := "git.tatikoma.dev"
pkg := hq + "/corpix/atlas"

goverter_cmd := """
goverter gen \
  -output-constraint '' \
  -g 'wrapErrors yes' \
  -g 'useZeroValueOnPointerInconsistency yes' \
  -g 'ignoreMissing no' \
  -g 'skipCopySameType yes' \
  -g 'ignoreUnexported yes' \
  -g 'matchIgnoreCase yes' \
  -g 'enum no'
"""

default:
  just test

build:
  go test -run '^$' ./...

lint:
  go vet ./...
  golangci-lint run -v

fmt: fmt-fieldalign fmt-go

fmt-fieldalign:
  betteralign -fix \
    $(go list -mod=mod all | grep -F {{pkg}} | grep -E -v '/pb(/.+)?$') || true

fmt-go:
  go fmt ./...

gen:
  cd rpc && buf generate --template buf.gen.yaml
  just fmt

conv:
  {{goverter_cmd}} ./...

test: lint
  go test -v ./...

tag:
  git tag "v$(date +"%Y-%m-%d").$(git rev-list --count HEAD)"
