# Dockerfile builds an image for a client_golang example.
#
# Use as (from the root for the client_golang repository):
#    docker build -f examples/$name/Dockerfile -t prometheus/golang-example-$name .

# Builder image, where we build the example.

FROM vi-nexus.lab.vi.local/golang:1.23.3 AS builder

ENV GOPATH /go

WORKDIR /nutanix-exporter
COPY . .
RUN echo "> GOPATH: " $GOPATH
RUN go get -d
RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags -w

# Final image.
FROM quay.io/prometheus/busybox:latest

LABEL description "Prometheus Exporter for Nutanix AHV Cluster" \
      version "v0.5.1" \
      maintainer "Martin Weber <martin.weber@de.clara.net>"

WORKDIR /
COPY --from=builder /nutanix-exporter/nutanix-exporter /usr/local/bin/nutanix-exporter
RUN mkdir /config && touch /config/config.yml

EXPOSE 9405
ENTRYPOINT ["/usr/local/bin/nutanix-exporter"]
CMD [ "--nutanix.conf", "/config/config.yml" ]
