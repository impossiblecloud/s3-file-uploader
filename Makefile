#
# Makefile
#
# Simple makefile to build stuff
#

VENDOR_DIR = vendor

.PHONY: get-deps
get-deps: $(VENDOR_DIR)

$(VENDOR_DIR):
	GO111MODULE=on go mod vendor

$(OUTPUT_DIR):
	mkdir output

.PHONY: build
build: $(VENDOR_DIR) $(OUTPUT_DIR)
	GOOS=linux CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o output/s3-file-uploader .

.PHONY: local-build
local-build: $(VENDOR_DIR) $(OUTPUT_DIR)
	CGO_ENABLED=1 go build -a -ldflags '-extldflags "-static"' -o output/s3-file-uploader .

.PHONY: local-build-wo-cgo
local-build-wo-cgo: $(VENDOR_DIR) $(OUTPUT_DIR)
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o output/s3-file-uploader .

.PHONY: clean
clean:
	rm -f output/*

.PHONY: clean-all
clean-all:
	rm -rf output/* vendor

.PHONY: test
test: $(VENDOR_DIR)
	go test -v -timeout 10s ./...
