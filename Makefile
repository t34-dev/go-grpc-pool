DEV_DIR := $(CURDIR)
APP_REPOSITORY := github.com/t34-dev
APP_NAME := go-grpc-pool
export GOPRIVATE=$(APP_REPOSITORY)/*

# includes
include .make/get-started.mk
include .make/tag.mk
include .make/protoc.mk
include .make/test.mk


protoc:
	@$(MAKE) --no-print-directory protoc-gen PROTO_FILE=example/api/exampleservice.proto

server:
	go run example/cmd/server/main.go

client:
	go run example/cmd/client/main.go
