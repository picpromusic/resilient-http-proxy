# Variables
CMD_DIR := ./cmd
BIN_DIR := $(GOPATH)/bin
GO := go

# Default target
.PHONY: all
all: install test

# Install all commands in the cmd directory
.PHONY: install
install:
	@echo "Installing all CLI tools..."
	$(GO) install $(CMD_DIR)/...

# Run tests
.PHONY: test
test: install
	@echo "Running tests..."
	$(GO) test ./... -v

# Clean up binaries
.PHONY: clean
clean:
	@echo "Cleaning up..."
	rm -f $(BIN_DIR)/randombackend
	rm -f $(BIN_DIR)/resilientproxy