FROM golang:latest AS gobuild

ARG GO111MODULE=on
WORKDIR ./src/noc-k8slabels-v1/container/go
COPY go.* ./
RUN go mod download

COPY pkg pkg/
COPY *.go .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -a -installsuffix cgo -o server .

FROM scratch
LABEL maintainer "ON2IT"
COPY --from=gobuild /go/src/noc-k8slabels-v1/container/go/server .
CMD ["/server"]

