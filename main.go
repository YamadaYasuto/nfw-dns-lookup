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

// 初期化関数
func init() {
	// 環境変数からドメインとルールグループ名を取得
	domain = os.Getenv("DOMAIN")
	statefulRuleGroupName = os.Getenv("STATEFUL_RULE_GROUP_NAME")

	// AWS 設定を読み込む
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		// エラーが発生した場合は、ログを出力してプログラムを終了
		log.Fatalf("Load AWS config failed: %v", err)
	}
	nfwClient = networkfirewall.NewFromConfig(cfg)
}

type Response struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

func handler(ctx context.Context) (Response, error) {

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
		resolved = append(resolved, ip+"/32")
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
		log.Printf("Describe rule group failed")
		return Response{}, err
	}
	log.Printf("Rule group retrieved successfully: %+v", out)

	// ルールグループのルール変数からIPアドレスを取得
	ruleVars := out.RuleGroup.RuleVariables
	existing := ruleVars.IPSets["IP_NET"].Definition

	// 3. DNSクエリ結果と既存のIPアドレスを比較

	// 更新がなければ終了
	if sameIPSet(resolved, existing) {
		log.Printf("No changes needed, skipping update")
		return Response{
			StatusCode: 200,
			Body:       "No changes needed",
		}, nil
	} else {
		// 更新があればIPアドレスを更新
		log.Printf("Replacing IP set: %v -> %v", existing, resolved)

		// DNSクエリ結果をIP_NETルール変数に設定
		ipSet := ruleVars.IPSets["IP_NET"]
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
	}

	body, _ := json.Marshal(fmt.Sprintf(
		"Domain %s successfully resolved and the Network Firewall rule group has been updated.",
		domain,
	))
	return Response{
		StatusCode: 200,
		Body:       string(body),
	}, nil
}

// 関数: 既存のIPアドレスとDNSクエリ結果を比較
func sameIPSet(resolved, existing []string) bool {

	if len(resolved) != len(existing) {
		return false
	}

	set := make(map[string]struct{}, len(resolved))
	for _, ip := range resolved {
		set[ip] = struct{}{}
	}
	for _, ip := range existing {
		if _, ok := set[ip]; !ok {
			return false
		}
	}

	return true
}

// エントリーポイント
func main() {
	lambda.Start(handler)
}
