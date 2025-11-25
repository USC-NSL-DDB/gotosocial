go mod vendor
go install github.com/ServiceWeaver/weaver/cmd/weaver@v0.22.0
weaver generate ./...
DEBUG=1 ./scripts/build.sh
