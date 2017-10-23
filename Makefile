OPERATOR_NAME  := hyperpilot-operator
VERSION := $(shell date +%Y%m%d%H%M)
IMAGE := ogre0403/$(OPERATOR_NAME)

.PHONY: install_deps build build-image

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

build-image:
	docker build -t $(IMAGE):$(VERSION) .
