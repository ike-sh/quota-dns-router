package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/master"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "help"
	if len(args) > 0 {
		cmd = args[0]
	}
	envPath := flagValue(args[1:], "--env", config.DefaultMasterEnvPath)
	switch cmd {
	case "run":
		cfg, err := config.LoadMaster(envPath)
		if err != nil {
			return err
		}
		return master.Run(context.Background(), cfg)
	case "telegram-run":
		cfg, err := config.LoadMaster(envPath)
		if err != nil {
			return err
		}
		return master.TelegramRun(context.Background(), cfg)
	case "migrate":
		cfg, err := config.LoadMaster(envPath)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		fmt.Println("migration 完成")
	case "status":
		cfg, err := config.LoadMaster(envPath)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		overview, err := master.BuildStatusOverview(context.Background(), store, cfg.PublicAPIURL, cloudflare.NewClient(nil), now())
		if err != nil {
			return err
		}
		fmt.Print(master.FormatStatusReport(overview.Setup, overview.Summary, overview.ReportExtras()))
	case "config-check":
		cfg, err := config.LoadMaster(envPath)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		overview, err := master.BuildStatusOverview(context.Background(), store, cfg.PublicAPIURL, cloudflare.NewClient(nil), now())
		if err != nil {
			return err
		}
		fmt.Print(formatMasterConfigCheck(cfg, overview))
	case "version":
		fmt.Println(version)
	default:
		printHelp()
	}
	return nil
}

func printHelp() {
	fmt.Println(`qdr-master commands:
  run [--env path]
  telegram-run [--env path]
  status [--env path]
  config-check [--env path]
  migrate [--env path]
  version`)
}

func flagValue(args []string, name, fallback string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return fallback
}

func now() time.Time {
	return time.Now()
}

func openStore(cfg config.MasterConfig) (*db.Store, error) {
	store, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func formatMasterConfigCheck(cfg config.MasterConfig, overview master.StatusOverview) string {
	status := overview.Setup
	out := fmt.Sprintf("配置检查通过：%s\nMaster Public API URL: %s\n", cfg.String(), status.PublicAPIURL)
	out += fmt.Sprintf("Cloudflare Token: %s\n", ternary(status.CloudflareTokenConfigured, "已配置 "+status.CloudflareTokenMasked, "未配置"))
	out += fmt.Sprintf("Zone: %s / %s\n", blankAsDash(status.ZoneName), blankAsDash(status.ZoneID))
	out += fmt.Sprintf("Zone 验证: %s\n", blankAsDash(overview.Cloudflare.Status))
	out += fmt.Sprintf("DNS A 记录配置数: %d\n", status.DNSConfigCount)
	out += fmt.Sprintf("分组数量: %d\n", status.GroupCount)
	out += fmt.Sprintf("节点数量: %d\n", status.NodeCount)
	out += fmt.Sprintf("在线 Agent 数量: %d\n", status.OnlineAgentCount)
	out += fmt.Sprintf("自动切换: %t\n", status.AutoSwitchEnabled)
	for _, item := range overview.DNS {
		out += fmt.Sprintf("DNS[%s]: %s 当前IP=%s 匹配节点=%s\n", item.GroupName, item.Status, blankAsDash(item.CurrentIP), blankAsDash(item.MatchedNodeName))
	}
	for _, group := range overview.Groups {
		out += fmt.Sprintf("Group[%s]: 可用切换目标=%d 状态=%s\n", group.Name, group.AvailableTargetCount, group.Status)
	}
	out += "\n最近切换:\n" + master.FormatRecentSwitchSummary(overview.RecentSwitch) + "\n"
	out += "\n最近失败:\n" + master.FormatRecentFailureSummary(overview.RecentFailure) + "\n"
	out += "\n当前风险:\n" + master.FormatStatusRiskSummary(overview.Risks) + "\n"
	if status.PublicURLWarning != "" {
		out += status.PublicURLWarning + "\n"
	}
	if len(status.Missing) > 0 {
		out += "缺少配置：" + strings.Join(status.Missing, "、") + "\n"
	}
	if warning := master.MasterPublicURLWarning(status.PublicAPIURL); warning != "" {
		out += warning + "\n"
		out += "提示：这会影响 Agent join/install，请先通过 Telegram /config_master_url 配置公网地址。\n"
	}
	return out
}

func ternary(ok bool, a, b string) string {
	if ok {
		return a
	}
	return b
}

func blankAsDash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}
