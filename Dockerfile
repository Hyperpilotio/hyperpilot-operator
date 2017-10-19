FROM golang:1.9

WORKDIR /go/src/github.com/lwolf/kube-cleanup-operator

RUN useradd -u 10001 kube-operator

RUN go get github.com/Masterminds/glide

COPY glide.yaml .
COPY glide.lock .

RUN glide install

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bin/linux/kube-cleanup-operator ./cmd


FROM alpine

RUN addgroup -S kube-operator && adduser -S -g kube-operator kube-operator

USER kube-operator

COPY --from=0 /go/src/github.com/lwolf/kube-cleanup-operator/bin/linux/kube-cleanup-operator .

ENTRYPOINT ["./kube-cleanup-operator"]



## Test OK!!
## first build using  GOOS=linux GOARCH=amd64 go build -v -i -o bin/linux/kube-cleanup-operator ./cmd
## Then docker build

#FROM alpine
#MAINTAINER Sergey Nuzhdin <ipaq.lw@gmail.com>
#
#RUN addgroup -S kube-operator && adduser -S -g kube-operator kube-operator
#
#USER kube-operator
#
#COPY bin/linux/kube-cleanup-operator .
#
#ENTRYPOINT ["./kube-cleanup-operator"]



