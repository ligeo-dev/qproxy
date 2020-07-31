PROJECT_NAME := "qproxy"
PKG := "github.com/empreinte-digitale/$(PROJECT_NAME)"

.PHONY: all dep lint build run clean

all: build

dep:
	@go get -v -d

lint: dep
	@golint -set_exit_status ./...

build: dep
	@go build -i -v $(PKG)

run: dep
	@go run main.go

clean:
	@rm -f $(PROJECT_NAME)

test: dep
	@go test ./...
