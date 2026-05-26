APP_NAME  = subset
BUILD_DIR = bin
DOCKER_COMPOSE ?= docker compose

SUBSET_TEST_DSN_POSTGRES ?= postgres://postgres:pass@127.0.0.1:5432/dev?sslmode=disable
SUBSET_TEST_DSN_MYSQL    ?= mysql://root:pass@127.0.0.1:3306/dev?parseTime=true

.PHONY: all build install uninstall clean test test-integration compose-up compose-down lint

all: build

build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) .

install:
	@echo "Installing $(APP_NAME)..."
	@bin_dir=$$(go env GOBIN); \
	if [ -z "$$bin_dir" ]; then \
		bin_dir=$$(go env GOPATH)/bin; \
	fi; \
	mkdir -p "$$bin_dir"; \
	echo "Installing to $$bin_dir/$(APP_NAME)"; \
	go build -o "$$bin_dir/$(APP_NAME)" .

uninstall:
	@echo "Uninstalling $(APP_NAME)..."
	@bin_dir=$$(go env GOBIN); \
	if [ -z "$$bin_dir" ]; then \
		bin_dir=$$(go env GOPATH)/bin; \
	fi; \
	echo "Removing $$bin_dir/$(APP_NAME)"; \
	rm -f "$$bin_dir/$(APP_NAME)"

clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)

test:
	go test ./... -race

test-integration:
	SUBSET_TEST_DSN_POSTGRES="$(SUBSET_TEST_DSN_POSTGRES)" \
	SUBSET_TEST_DSN_MYSQL="$(SUBSET_TEST_DSN_MYSQL)" \
	go test -tags=integration ./... -race $(GOTESTFLAGS)

compose-up:
	$(DOCKER_COMPOSE) up -d --wait

compose-down:
	$(DOCKER_COMPOSE) down

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint is not installed"; \
		exit 1; \
	}
	golangci-lint run
