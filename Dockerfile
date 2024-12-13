#
# Simple tool to watch directory for new files and upload them to S3
#

FROM golang:1.22.10 AS test
ENV GOPATH=/go
ENV PATH="$PATH:$GOPATH/bin"
COPY . ./
RUN make test

FROM test AS build
ENV GOPATH=/go
ENV PATH="$PATH:$GOPATH/bin"
RUN make build

FROM gcr.io/distroless/base-debian11
COPY --from=build ./output/s3-file-uploader /s3-file-uploader
ENTRYPOINT ["/s3-file-uploader"]
