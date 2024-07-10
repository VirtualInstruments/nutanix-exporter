
BIN_NAME = nutanix-exporter
DOCKER_IMAGE_NAME ?= nutanix-exporter
#export GOPATH = ${PWD}
export CGO_ENABLED = 0
export GOBUILD_ARGS = -a -tags netgo -ldflags -w
export GOARCH ?= amd64
# export GOOS ?= linux

all: linux

linux: prepare
	$(eval export GOOS=linux)
	go build $(GOBUILD_ARGS) -o ./bin/$(BIN_NAME)

clean:
	@echo "Clean up"
	go clean
	rm -rf bin/

docker:
	@echo ">> Compile using docker container"
	@docker build -t "$(DOCKER_IMAGE_NAME)" .

prepare:	
	@echo "Create output directory ./bin/"
	go env
	mkdir -p bin/
	@echo "GO get dependencies"
	go get -d
	

.PHONY: all
