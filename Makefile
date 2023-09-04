VERSION = v0.1.0
GIT_DIR = github.com/KubeSphereCloud/resource-quota-initiator
GIT_COMMIT=$(shell git rev-parse HEAD | head -c 7)
GIT_BRANCH=$(shell git name-rev --name-only HEAD)
BUILD_DATE=$(shell date '+%Y-%m-%d-%H:%M:%S')

IMG ?= stoneshiyunify/quota-manager:${GIT_COMMIT}
IMGLATEST ?= stoneshiyunify/quota-manager:latest

.PHONY: all
all: buildgo

.PHONY: build
build:
	go mod tidy && go mod verify && go build -o bin/manager main.go

.PHONY: docker-build
docker-build: #test ## Build docker image with the manager.
	docker build -t ${IMG} --build-arg VERSION=${VERSION} --build-arg GIT_DIR=${GIT_DIR} --build-arg GIT_COMMIT=${GIT_COMMIT} --build-arg GIT_BRANCH=${GIT_BRANCH} --build-arg BUILD_DATE=${BUILD_DATE} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}
	docker tag ${IMG} ${IMGLATEST}
	docker push ${IMGLATEST}