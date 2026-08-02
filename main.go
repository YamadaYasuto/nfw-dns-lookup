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

// パッケージ変数
var (
	domain                string // ドメイン名
	statefulRuleGroupName string // NetworkFirewallのルールグループ名
	nfwClient             *networkfirewall.Client // NetworkFirewallのクライアント
)

// エントリーポイント
func main() {

	// 環境変数からドメインとルールグループ名を取得
	domain = os.Getenv("DOMAIN")
	statefulRuleGroupName = os.Getenv("STATEFUL_RULE_GROUP_NAME")
	if domain == "" || statefulRuleGroupName == "" {
		log.Fatalf("DOMAIN and STATEFUL_RULE_GROUP_NAME must be set")
	}

	// AWS 設定を読み込む
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		// エラーが発生した場合は、ログを出力してプログラムを終了
		log.Fatalf("Load AWS config failed: %v", err)
	}
	nfwClient = networkfirewall.NewFromConfig(cfg)

	// ハンドラーを起動
	lambda.Start(handler)
}


// ハンドラー関数
// ドメインに対応するIPアドレスを取得し、NetworkFirewallに設定しているIPアドレスと比較し、
// 異なる場合はNetworkFirewallに設定しているIPアドレスを更新する
func handler(ctx context.Context) error {

	// 1. DNSクエリでIPアドレスを取得

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

	// 有効なIPv4アドレスが一つもなければエラーを返す
	if len(resolved) == 0 {
		return fmt.Errorf("no valid IPv4 addresses found %s", domain)
	}

	// 2. 既存のNetworkFirewallに設定しているIPアドレスを取得

	// ルールグループの取得に必要なパラメータを設定
	describeParams := networkfirewall.DescribeRuleGroupInput{
		RuleGroupName: aws.String(statefulRuleGroupName),
		Type:          types.RuleGroupTypeStateful,
	}

	// ルールグループを取得
	out, err := nfwClient.DescribeRuleGroup(ctx, &describeParams)
	if err != nil {
		return fmt.Errorf("Describe rule group failed: %w", err)
	}

	// ルールグループのルール変数からIPアドレスを取得
	if out.RuleGroup == nil {
		return fmt.Errorf("Rule group not found")
	}
	ruleVars := out.RuleGroup.RuleVariables
	if ruleVars == nil || ruleVars.IPSets == nil {
		return fmt.Errorf("Rule group has no RuleVariables or IPSets")
	}
	ipSet, ok := ruleVars.IPSets["IP_NET"]
	if !ok {
		return fmt.Errorf("IP_NET IPSet not found in RuleVariables.IPSets")
	}
	existing := ipSet.Definition
	log.Printf("Existing IPs: %v", existing)

	// 3. DNSクエリ結果と既存のIPアドレスを比較

	// 差分がなければ更新をスキップして終了
	if sameIPSet(resolved, existing) {
		log.Printf("No changes needed, skipping update")
		return nil
	}

	// IPアドレスを更新
	log.Printf("Replacing IP set: %v -> %v", existing, resolved)

	// DNSクエリ結果をIP_NETルール変数に設定
	ipSet.Definition = resolved
	ruleVars.IPSets["IP_NET"] = ipSet

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

	// IP_NETルール変数を更新(インプレース)
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
// 既存のIPアドレスとDNSクエリ結果が同じ場合はtrueを返す 異なればfalseを返す
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
