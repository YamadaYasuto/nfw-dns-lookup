# nfw-dns-lookup

特定ドメインの DNS を解決し、AWS Network Firewall の Stateful Rule Group（`IP_NET`）を IPv4 の結果で更新する Lambda 関数用のコードです。
AWSのNetwork Firewallを使って、non-HTTPS/non-HTTP通信のドメインフィルタリングを行いたい場合、このソリューションを利用できます。

## 前提環境

### Windows の場合

本リポジトリのビルド・テストは **Linux 前提** です。  
Windows の場合は、**WSL2** 上で作業してください。

1. PowerShell（管理者）で WSL をインストールします。

```powershell
wsl --install
```

1. 再起動後、Ubuntu などのディストリビューションを起動します。
2. 以降の作業は **すべて WSL 内のターミナル** で行ってください。

詳細は [Microsoft の WSL インストール手順](https://learn.microsoft.com/ja-jp/windows/wsl/install) を参照してください。

## 必要な Go バージョン

Go **1.25.6** 以上をインストールしてください。  
（このリポジトリの開発環境は 1.25.6です）



## Go のインストール（WSL / Ubuntu） 2026/07/29時点

```bash
# 古い Go があれば削除
sudo rm -rf /usr/local/go

# ダウンロード（amd64 の例）
curl -LO https://go.dev/dl/go1.25.6.linux-amd64.tar.gz

# 展開
sudo tar -C /usr/local -xzf go1.25.6.linux-amd64.tar.gz

# PATH を通す
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 確認
go version
```


## ビルド・テスト（Makefile）

依存関係の整理・テスト・ビルドは **Makefile を参照** してください。定義の詳細は `[Makefile](./Makefile)` にあります。

```bash
cd nfw-dns-lookup

make tidy      # 依存関係の整理（go mod tidy）
make test      # 単体テスト
make build     # ローカル用バイナリ
make package   # Lambda 用 bootstrap + function.zip
make clean     # 成果物の削除
```

`make package` は内部で `tidy` と `test` を実行したあと、Linux 向けにクロスコンパイルします。



## Lambda 環境変数


| 名前                         | 説明                          |
| -------------------------- | --------------------------- |
| `DOMAIN`                   | 解決するドメイン名                   |
| `STATEFUL_RULE_GROUP_NAME` | 更新対象の Stateful Rule Group 名 |



## Lambda ランタイム


| 項目           | 値                           |
| ------------ | --------------------------- |
| Runtime      | `provided.al2023`           |
| エントリポイント     | `bootstrap`                 |
| Architecture | `x86_64`（`make package` 想定） |


