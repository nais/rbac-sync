NAME := rbac-sync
REPO := navikt/${NAME}
TAG := $(shell date +'%Y-%m-%d')-$(shell git rev-parse --short HEAD)

.PHONY: test install build docker-build docker-push

test:
	go test

install:
	go get -u k8s.io/client-go/...
	go get -u github.com/prometheus/client_golang/...
	go get -u golang.org/x/oauth2/...
	go get -u google.golang.org/api/groupssettings/v1
	go get -u google.golang.org/api/admin/directory/v1
	go get -u github.com/sirupsen/logrus

build:
	go build -o ${NAME}

docker-build:
	docker build -t "$(REPO):$(TAG)" -t "$(REPO):latest" .

docker-push:
	docker push "$(REPO)"
