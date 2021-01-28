FROM golang:1.15-alpine

RUN apk add --update make

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# https://github.com/tektoncd/pipeline/blob/master/docs/auth.md#using-secrets-as-a-non-root-user
ENV HOME=/tekton/home
RUN mkdir -p $HOME
