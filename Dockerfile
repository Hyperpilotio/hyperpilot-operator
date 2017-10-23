FROM golang:1.9

WORKDIR /go/src/github.com/hyperpilotio/hyperpilot-operator

RUN useradd -u 10001 kube-operator

RUN go get github.com/Masterminds/glide

COPY . .

RUN make install_deps
RUN make build-in-docker


FROM alpine

RUN addgroup -S kube-operator && adduser -S -g kube-operator kube-operator

USER kube-operator

COPY --from=0 /go/src/github.com/hyperpilotio/hyperpilot-operator/bin/linux/hyperpilot-operator .

CMD ["./hyperpilot-operator"]