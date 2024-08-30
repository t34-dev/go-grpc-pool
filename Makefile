APP_NAME := go-grpc-pool
APP_REPOSITORY := github.com/t34-dev
%:
	@:
MAKEFLAGS += --no-print-directory
export GOPRIVATE=$(APP_REPOSITORY)/*
export COMPOSE_PROJECT_NAME
# ============================== Environments
ENV ?= local
FOLDER ?= /root
VERSION ?=
# ============================== Paths
APP_EXT := $(if $(filter Windows_NT,$(OS)),.exe)
BIN_DIR := $(CURDIR)/.bin
DEVOPS_DIR := $(CURDIR)/.devops
ENV_FILE := $(CURDIR)/.env
SECRET_FILE := $(CURDIR)/.secrets
CONFIG_DIR := $(CURDIR)/configs
# ============================== Includes
include .make/get-started.mk
include .make/tag.mk
include .make/proto.mk
include .make/test.mk


server:
	go run example/cmd/server/main.go

client:
	go run example/cmd/client/main.go
