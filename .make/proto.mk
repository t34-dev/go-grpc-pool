proto-plugin:
	GOBIN=$(BIN_DIR) go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1
	GOBIN=$(BIN_DIR) go install -mod=mod google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

proto-vendor:
		@if [ ! -d example/api/google ]; then \
			git clone https://github.com/googleapis/googleapis example/api/googleapis &&\
			mkdir -p  example/api/google/ &&\
			mv example/api/googleapis/google/api example/api/google &&\
			rm -rf example/api/googleapis ;\
		fi

proto:
	@$(MAKE) proto-example

proto-example:
	@mkdir -p example/pkg/api
	@protoc --proto_path example/api \
		--go_out=example/pkg/api --go_opt=paths=source_relative \
			--plugin=protoc-gen-go=$(BIN_DIR)/protoc-gen-go$(APP_EXT) \
		--go-grpc_out=example/pkg/api --go-grpc_opt=paths=source_relative \
			--plugin=protoc-gen-go-grpc=$(BIN_DIR)/protoc-gen-go-grpc$(APP_EXT) \
		example/api/example_v1/example.proto
	@echo "Done"

proto-test-random:
	$(BIN_DIR)/grpcurl$(APP_EXT) -plaintext \
		-proto api/random_v1/random.proto \
		-import-path ./api \
		-d '{}' \
		127.0.0.1:50051 \
		random_v1.RandomService/GetPing


.PHONY: proto-plugin proto-vendor proto proto-random proto-test
