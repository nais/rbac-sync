NAME := rbac-sync
REPO := navikt/${NAME}
TAG := $(shell date +'%Y-%m-%d')-$(shell git rev-parse --short HEAD)

.PHONY: test install build docker-build docker-push

test:
	go test

install:
	go install

build:
	go build -o ${NAME}

docker-build:
	docker build -t "$(REPO):$(TAG)" -t "$(REPO):latest" .

docker-push:
	docker push "$(REPO)"
