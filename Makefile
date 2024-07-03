
BIN_NAME = nutanix-exporter
DOCKER_IMAGE_NAME ?= nutanix-exporter
#export GOPATH = ${PWD}
export CGO_ENABLED = 0
export GOBUILD_ARGS = -a -tags netgo -ldflags -w
export GOARCH ?= amd64
# export GOOS ?= linux

all: linux windows docker

linux: prepare
	$(eval export GOOS=linux)
	go build $(GOBUILD_ARGS) -o ./bin/$(BIN_NAME)
	zip ./bin/$(BIN_NAME)-$(GOOS)-$(GOARCH).zip ./bin/$(BIN_NAME)

clean:
	@echo "Clean up"
	go clean
	rm -rf bin/

docker:
	@echo ">> Compile using docker container"
	@docker build -t "$(DOCKER_IMAGE_NAME)" .
	@echo $NEXUS_PASSWORD | docker login -u $NEXUS_USER --password-stdin $NEXUS_SERVER
	@echo $AWS_PASSWORD | docker login -u AWS --password-stdin AWS_ECR_REGISTRY
	@docker tag $(DOCKER_IMAGE_NAME):$(TAG) $(NEXUS_SERVER)/$(DOCKER_IMAGE_NAME):$(TAG)
	@docker push $(NEXUS_SERVER)/$(DOCKER_IMAGE_NAME):$(TAG) 
	@docker tag $(DOCKER_IMAGE_NAME):$(TAG) $(AWS_ECR_REGISTRY)/$(DOCKER_IMAGE_NAME):$(TAG)
	@docker push $(AWS_ECR_REGISTRY)/$(DOCKER_IMAGE_NAME):$(TAG) 

windows: prepare
	$(eval export GOOS=windows)
	go build $(GOBUILD_ARGS) -o ./bin/$(BIN_NAME).exe
	zip ./bin/$(BIN_NAME)-$(GOOS)-$(GOARCH).zip ./bin/$(BIN_NAME).exe

prepare:	
	@echo "Create output directory ./bin/"
	go env
	mkdir -p bin/
	@echo "GO get dependencies"
	go get -d
	

.PHONY: all
