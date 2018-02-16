OPERATOR_NAME  := hyperpilot-operator
IMAGE := hyperpilot/$(OPERATOR_NAME)
VERSION := hosted-service

TEST_IMAGE := ogre0403/$(OPERATOR_NAME)
TEST_VERSION := hosted-service

.PHONY: install_deps build build-ubuntu-image

install_deps:
	glide install

build:
	rm -rf bin/*
	go build -v -i -o bin/$(OPERATOR_NAME) ./cmd

build-in-docker:
	rm -rf bin/*
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/linux/$(OPERATOR_NAME) ./cmd

clean:
	rm -rf bin/*

build-alpine-image:
	docker build -t $(IMAGE):$(VERSION)-alpine -f ./dockerfiles/alpine/Dockerfile .

build-ubuntu-image:
	docker build -t $(IMAGE):$(VERSION)-ubuntu -f ./dockerfiles/ubuntu/Dockerfile .

build-test-image:
	docker build -t $(TEST_IMAGE):$(TEST_VERSION) -f ./dockerfiles/ubuntu/Dockerfile .


