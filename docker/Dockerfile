FROM alpine

RUN apk add --no-cache curl

CMD ["/bin/sh", "-c", "curl -H 'Metadata-Flavor: Google' http://metadata.google.internal/computeMetadata/v1/instance/machine-type"]