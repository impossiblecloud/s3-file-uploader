#
# Simple tool to watch directory for new files and upload them to S3
#

FROM golang:1.22.10 AS test
WORKDIR /build
ENV GOPATH=/go
ENV PATH="$PATH:$GOPATH/bin"
COPY Makefile Makefile
COPY main.go main.go
COPY main_test.go main_test.go
COPY go.mod go.mod
COPY go.sum go.sum
COPY internal/ internal/
RUN make test

FROM test AS build
WORKDIR /build
ENV GOPATH=/go
ENV PATH="$PATH:$GOPATH/bin"
RUN make build

# FROM gcr.io/distroless/base-debian11
FROM alpine:3.21
WORKDIR /
COPY --from=build /build/output/s3-file-uploader /s3-file-uploader
RUN apk add --no-cache inotify-tools gpg && \
    mkdir -p /app/enc /app/tmp /app/gzip
ENTRYPOINT ["/s3-file-uploader"]
