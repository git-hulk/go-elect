CCCOLOR="\033[37;1m"
MAKECOLOR="\033[32;1m"
ENDCOLOR="\033[0m"

lint:
	@printf $(CCCOLOR)"Running linter...\n"$(ENDCOLOR)
	@golangci-lint run

test:
	@printf $(CCCOLOR)"Running tests...\n"$(ENDCOLOR)
	@sh ./scripts/setup.sh
	@go test -v ./... -race
	@sh ./scripts/teardown.sh
