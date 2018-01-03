FROM golang:1.9

WORKDIR /go/src/github.com/hyperpilotio/hyperpilot-operator

RUN useradd -u 10001 kube-operator

RUN go get github.com/Masterminds/glide

COPY . .

RUN make install_deps
RUN make build-in-docker


FROM alpine

RUN addgroup -S kube-operator && adduser -S -g kube-operator kube-operator
RUN mkdir -p /etc/operator && chown kube-operator:kube-operator /etc/operator
RUN apk update && apk add ca-certificates

USER kube-operator

COPY --from=0 /go/src/github.com/hyperpilotio/hyperpilot-operator/bin/linux/hyperpilot-operator .
COPY --from=0 /go/src/github.com/hyperpilotio/hyperpilot-operator/conf/operator_config.json /etc/operator
CMD ["./hyperpilot-operator"]