package main

import (
	"context"
	"encoding/json"
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

var (
	domain                string
	statefulRuleGroupName string
	nfwClient             *networkfirewall.Client
)

type Response struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

func handler(ctx context.Context) (Response, error) {

	// 0. 環境変数とAWS設定を読み込む

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

	// 1. DNSクエリでIPアドレスを取得

	// DNSクエリを実行
	ips, err := net.LookupHost(domain)
	if err != nil {
		log.Printf("Dns lookup failed: %v", err)
		return Response{}, err
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
		log.Printf("The Domain: %s doesn't have any valid IPv4 addresses", domain)
		return Response{}, fmt.Errorf("no valid IPv4 addresses found %s", domain)
	}

	// 2. 既存のNetworkFirewallに設定しているIPアドレスを取得

	// ルールグループの取得に必要なパラメータを設定
	decribeParams := networkfirewall.DescribeRuleGroupInput{
		RuleGroupName: aws.String(statefulRuleGroupName),
		Type:          types.RuleGroupTypeStateful,
	}

	// ルールグループを取得
	out, err := nfwClient.DescribeRuleGroup(ctx, &decribeParams)
	if err != nil {
		log.Printf("Describe rule group failed: %v", err)
		return Response{}, err
	}

	// ルールグループのルール変数からIPアドレスを取得
	if out.RuleGroup == nil {
		return Response{}, fmt.Errorf("rule group not found")
	}
	ruleVars := out.RuleGroup.RuleVariables
	if ruleVars == nil || ruleVars.IPSets == nil {
		return Response{}, fmt.Errorf("rule group has no RuleVariables or IPSets")
	}
	ipSet, ok := ruleVars.IPSets["IP_NET"]
	if !ok {
		return Response{}, fmt.Errorf("IP_NET IPSet not found in RuleVariables.IPSets")
	}
	existing := ipSet.Definition
	log.Printf("Existing IPs: %v", existing)

	// 3. DNSクエリ結果と既存のIPアドレスを比較

	// 更新がなければ終了
	if sameIPSet(resolved, existing) {
		log.Printf("No changes needed, skipping update")
		return Response{
			StatusCode: 200,
			Body:       "No changes needed",
		}, nil
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
		log.Printf("Update rule group failed: %v", err)
		return Response{}, err
	}
	log.Printf("Rule group updated successfully")

	body, _ := json.Marshal(fmt.Sprintf(
		"Domain %s successfully resolved and the Network Firewall rule group has been updated.",
		domain,
	))
	return Response{
		StatusCode: 200,
		Body:       string(body),
	}, nil
}

// ヘルパー関数: IPv4のバリデーション
func isValidIPv4(s string) bool {

	// IPv4 or IPv6ではない場合はnilを返す
	ip := net.ParseIP(s)

	// IPv4以外はfalseを返す
	return ip != nil && ip.To4() != nil
}

// ヘルパー関数関数: 既存のIPアドレスとDNSクエリ結果を比較
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

// エントリーポイント
func main() {
	lambda.Start(handler)
}
