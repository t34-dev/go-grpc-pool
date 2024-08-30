test:
	@mkdir -p $(DEVOPS_DIR)/.temp
	@CGO_ENABLED=0 go test \
	. \
	-coverprofile=$(DEVOPS_DIR)/.temp/coverage-report.out -covermode=count
	@go tool cover -html=$(DEVOPS_DIR)/.temp/coverage-report.out -o $(DEVOPS_DIR)/.temp/coverage-report.html
	@go tool cover -func=$(DEVOPS_DIR)/.temp/coverage-report.out


.PHONY: test
