FROM golang:1.12.6 as builder
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GO111MODULE=on
WORKDIR /go/src/github.com/nais/rbac-sync
COPY . .
RUN make install; go build -o rbac-sync; \
  curl -o ca-certificates.crt https://curl.haxx.se/ca/cacert.pem;

FROM scratch
MAINTAINER Sten Røkke <sten.ivar.rokke@nav.no>
COPY --from=builder /go/src/github.com/nais/rbac-sync/rbac-sync /app/rbac-sync
COPY --from=builder /go/src/github.com/nais/rbac-sync/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENTRYPOINT ["/app/rbac-sync"]
