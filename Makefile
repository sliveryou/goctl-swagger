.PHONY: proxy tidy fmt lint pre-commit

proxy:
	@go env -w GO111MODULE="on"
	@go env -w GOPROXY="https://goproxy.cn,direct"

tidy:
	@go mod tidy -e -v

fmt:
	@find . -name '*.go' -not -path "./example/*" | xargs gofumpt -w -extra
	@find . -name '*.go' -not -path "./example/*" | xargs -n 1 -t goimports-reviser -rm-unused -set-alias -company-prefixes "github.com/sliveryou" -project-name "github.com/sliveryou/goctl-swagger"
	@find . -name '*.sh' -not -path "./example/*" | xargs shfmt -w -s -i 2 -ci -bn -sr

lint:
	@golangci-lint run ./...

pre-commit: tidy fmt lint
