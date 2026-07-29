.PHONY: tidy test build clean package

# 依存関係の整理
tidy:
	go mod tidy

# 単体テスト
test:
	go test -v ./...

# ローカル確認用ビルド
build:
	go build -o bin/nfw-dns-lookup .

# Lambda デプロイ用（provided.al2023 / x86_64）
package: tidy test
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap .
	zip -q function.zip bootstrap
	@echo "created: bootstrap, function.zip"

# 成果物削除
clean:
	rm -f bootstrap function.zip
	rm -rf bin