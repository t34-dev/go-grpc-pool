################################################################## protoc
protoc-gen:
	@if [ -z "$(PROTO_FILE)" ]; then \
		echo "Usage: make protoc-gen PROTO_FILE=<path_to_proto_file>"; \
		exit 1; \
	fi
	@if [ ! -f "$(PROTO_FILE)" ]; then \
		echo "Error: File $(PROTO_FILE) does not exist"; \
		exit 1; \
	fi
	@protoc --go_out=. --go_opt=module=$(APP_REPOSITORY)/$(APP_NAME) \
    		   --go-grpc_out=. --go-grpc_opt=module=$(APP_REPOSITORY)/$(APP_NAME) \
    		   $(PROTO_FILE)
	@echo "Generated files:"
	@find . -newer $(PROTO_FILE) -name "*.pb.go" -o -name "*.grpc.pb.go"


.PHONY: protoc-gen
