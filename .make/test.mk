test:
	@mkdir -p $(TEMP_DIR)/.temp
	@CGO_ENABLED=0 go test \
	. \
	-coverprofile=$(TEMP_DIR)/.temp/coverage-report.out -covermode=count
	@go tool cover -html=$(TEMP_DIR)/.temp/coverage-report.out -o $(TEMP_DIR)/.temp/coverage-report.html
	@go tool cover -func=$(TEMP_DIR)/.temp/coverage-report.out


.PHONY: test
