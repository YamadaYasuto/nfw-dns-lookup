package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	"github.com/aws/aws-sdk-go-v2/service/networkfirewall/types"
)

// main関数で初期化する変数
// これらの変数はハンドラー関数から参照できるようにする
var (
	domain                string
	statefulRuleGroupName string
	nfwClient             *networkfirewall.Client
)

// エントリーポイント
// 環境変数からドメインとルールグループ名を取得
// 起動時に一度だけ実行されるようにしてオーバーヘッドを削減する
func main() {
	domain = os.Getenv("DOMAIN")
	statefulRuleGroupName = os.Getenv("STATEFUL_RULE_GROUP_NAME")

	// ドメインとルールグループ名が設定されていない場合は、ログを出力してプログラムを終了
	if domain == "" || statefulRuleGroupName == "" {
		log.Fatalf("DOMAIN and STATEFUL_RULE_GROUP_NAME must be set")
	}

	// AWS 設定を読み込む
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		// エラーが発生した場合は、ログを出力してプログラムを終了
		log.Fatalf("Load AWS config failed: %v", err)
	}

	// NetworkFirewallのクライアントを初期化
	nfwClient = networkfirewall.NewFromConfig(cfg)

	// ハンドラー関数の登録と、呼び出し処理の開始
	lambda.Start(handler)
}

// ハンドラー関数
// 1. ドメインに対応するIPアドレスを取得
// 2. NetworkFirewallに設定しているルール変数のIPアドレスを取得
// 3. 比較して差分があれば更新。なければ更新せず終了
func handler(ctx context.Context) error {

	// --- 1. ドメインに対応するIPアドレスを取得 ---

	// DNSクエリを実行
	ips, err := net.LookupHost(domain)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %w", err)
	}
	log.Printf("Domain maps to following IPs: %v", ips)

	// cidr表記に変換
	resolved := make([]string, 0, len(ips))
	for _, ip := range ips {
		// IPv4以外はスキップ
		if !isValidIPv4(ip) {
			log.Printf("Skipping non-IPv4 address: %s", ip)
			continue
		}
		resolved = append(resolved, ip+"/32")
	}

	// 有効なIPv4アドレスがない場合、更新するとホワイトリストが空になるためエラーを返す
	if len(resolved) == 0 {
		return fmt.Errorf("no valid IPv4 addresses found for domain: %s", domain)
	}

	// --- 2. NetworkFirewallに設定しているルール変数のIPアドレスを取得 ---

	// ルールグループ情報の取得に必要なパラメータを設定
	describeParams := networkfirewall.DescribeRuleGroupInput{
		RuleGroupName: aws.String(statefulRuleGroupName),
		Type:          types.RuleGroupTypeStateful,
	}

	// ルールグループ情報を取得
	out, err := nfwClient.DescribeRuleGroup(ctx, &describeParams)
	if err != nil {
		return fmt.Errorf("Describe rule group failed: %w", err)
	}

	// ルールグループのルール変数からIP_NETのIPアドレスを取得
	// nil参照の場合、panicではなく明示的にエラーを返す
	if out.RuleGroup == nil {
		return fmt.Errorf("Rule group not found")
	}
	ruleVars := out.RuleGroup.RuleVariables
	if ruleVars == nil || ruleVars.IPSets == nil {
		return fmt.Errorf("Rule group has no RuleVariables or IPSets")
	}
	whitelist, ok := ruleVars.IPSets["IP_NET"]
	// IP_NETが設定されていない場合、エラーを返す
	if !ok {
		return fmt.Errorf("IP_NET IPSet not found in RuleVariables.IPSets")
	}
	existing := whitelist.Definition
	log.Printf("Existing IPs: %v", existing)

	// --- 3. 比較して差分があれば更新。なければ更新せず終了 ---

	// 差分がなければ更新をスキップして終了
	if sameIPSet(resolved, existing) {
		log.Printf("No changes needed, skipping update")
		return nil
	}

	log.Printf("Replacing IP set: %v -> %v", existing, resolved)

	// DNSクエリ結果をIP_NETルール変数に設定
	// IPSetsはmap[string]types.IPSetのため、一度IPSet型の変数(whitelist)に書き込み、その後mapに代入する
	// 注意）map内のstructのfieldは直接代入できない
	// e.g., ruleVars.IPSets["IP_NET"].Definition = resolved // コンパイルエラー
	whitelist.Definition = resolved
	ruleVars.IPSets["IP_NET"] = whitelist

	// ルールグループの更新に必要なパラメータを設定
	updateParams := networkfirewall.UpdateRuleGroupInput{
		UpdateToken:   out.UpdateToken,
		RuleGroupArn:  out.RuleGroupResponse.RuleGroupArn,
		RuleGroupName: aws.String(statefulRuleGroupName),
		RuleGroup: &types.RuleGroup{
			RuleVariables: ruleVars,
			RulesSource:   out.RuleGroup.RulesSource,
		},
		Type:   types.RuleGroupTypeStateful,
		DryRun: false,
	}

	// IP_NETルール変数を更新
	_, err = nfwClient.UpdateRuleGroup(ctx, &updateParams)
	if err != nil {
		return fmt.Errorf("Update rule group failed: %w", err)
	}
	log.Printf("Rule group updated successfully")

	return nil
}

// ヘルパー関数: IPv4のバリデーション
// 有効なIPv4アドレスの場合はtrueを返す 無効な場合はfalseを返す
func isValidIPv4(s string) bool {

	// net.ParseIPでIPアドレスを解析
	ip := net.ParseIP(s)

	return ip != nil && ip.To4() != nil
}

// ヘルパー関数: 既存のIPアドレスとDNSクエリ結果を比較
// 同じ場合はtrue、異なればfalseを返す
// なお、resolvedとexistingの順番は考慮しない
func sameIPSet(resolved, existing []string) bool {

	// resolvedのIPアドレスの重複を削除
	setResolved := make(map[string]struct{}, len(resolved))
	for _, ip := range resolved {
		setResolved[ip] = struct{}{}
	}

	// existingのIPアドレスの重複を削除
	setExisting := make(map[string]struct{}, len(existing))
	for _, ip := range existing {
		setExisting[ip] = struct{}{}
	}

	// resolvedとexistingのIPアドレスの数が異なればfalse
	if len(setResolved) != len(setExisting) {
		return false
	}

	// 比較
	for ip := range setResolved {
		if _, ok := setExisting[ip]; !ok {
			return false
		}
	}

	return true
}
