# sya — git-native issue tracker for AI-agent workflows

binary := "sya"
version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X main.version=" + version

# default recipe: build for the host platform
default: build

# build for the host platform into bin/
build:
    go build -trimpath -ldflags "{{ ldflags }}" -o bin/{{ binary }} ./cmd/sya

# install into GOBIN (or GOPATH/bin)
install:
    go install -trimpath -ldflags "{{ ldflags }}" ./cmd/sya

# run all tests
test:
    go test ./... -count=1 -timeout 120s

# run black-box functional CLI tests
func:
    go test ./tests/functional -count=1

# run real-git integration CLI tests
integ:
    go test ./tests/integration -count=1

# go vet + gofmt check
lint:
    go vet ./...
    test -z "$(gofmt -l .)"

validate-plugin:
    claude plugin validate .
    claude plugin validate claude-plugin

bump-plugin-version version:
    sh scripts/bump-plugin-version.sh {{ version }}

fmt:
    gofmt -w .

clean:
    rm -rf bin dist

# cross-compile release binaries for linux and macos (amd64 + arm64)
release:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p dist
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
        goos="${target%/*}" goarch="${target#*/}"
        out="dist/{{ binary }}-${goos}-${goarch}"
        echo "building ${out}"
        GOOS="${goos}" GOARCH="${goarch}" CGO_ENABLED=0 \
            go build -trimpath -ldflags "{{ ldflags }}" -o "${out}" ./cmd/sya
    done

# print the version that would be embedded
version:
    @echo {{ version }}
