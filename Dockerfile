FROM golang:1.18-alpine as builder

ARG VERSION

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY cmd/ cmd/
COPY pkg/ pkg/
COPY Makefile Makefile

RUN apk add --update make

RUN make build VERSION=$VERSION

FROM scratch

COPY --from=builder workspace/bin/kfmt /kfmt

ENTRYPOINT ["/kfmt"]
