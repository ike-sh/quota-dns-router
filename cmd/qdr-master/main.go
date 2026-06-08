package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/master"
	"quota-dns-router-go/internal/version"
)

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
	cfgPath, cfgPathProvided := configFlag(args[1:], config.DefaultMasterEnvPath)
	switch cmd {
	case "run":
		loadPath := ""
		if cfgPathProvided {
			loadPath = cfgPath
		}
		cfg, err := config.LoadMaster(loadPath)
		if err != nil {
			return err
		}
		return master.Run(context.Background(), cfg)
	case "telegram-run":
		fmt.Fprintln(os.Stderr, "警告：telegram-run 仅启动 Telegram Bot，不启动 HTTP API、离线检查与自动切换。生产环境请使用 qdr-master run。")
		loadPath := ""
		if cfgPathProvided {
			loadPath = cfgPath
		}
		cfg, err := config.LoadMaster(loadPath)
		if err != nil {
			return err
		}
		return master.TelegramRun(context.Background(), cfg)
	case "migrate":
		cfg, err := config.LoadMaster(cfgPath)
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
		cfg, err := config.LoadMaster(cfgPath)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		dns, err := master.NewDNSProvider(cfg.DNSProvider, cfg.AWSRegion)
		if err != nil {
			return err
		}
		overview, err := master.BuildStatusOverview(context.Background(), store, cfg.PublicAPIURL, dns, now())
		if err != nil {
			return err
		}
		fmt.Print(master.FormatStatusReport(overview.Setup, overview.Summary, overview.ReportExtras()))
	case "config-check":
		cfg, err := config.LoadMaster(cfgPath)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		dns, err := master.NewDNSProvider(cfg.DNSProvider, cfg.AWSRegion)
		if err != nil {
			return err
		}
		overview, err := master.BuildStatusOverview(context.Background(), store, cfg.PublicAPIURL, dns, now())
		if err != nil {
			return err
		}
		fmt.Print(formatMasterConfigCheck(cfg, overview))
	case "telegram-status":
		cfg, err := config.LoadMaster(cfgPath)
		if err != nil {
			return err
		}
		status, err := master.BuildTelegramStatus(context.Background(), cfg)
		if err != nil {
			return err
		}
		fmt.Print(master.FormatTelegramStatus(status))
	case "backup":
		cfg, err := config.LoadMaster(cfgPath)
		if err != nil {
			return err
		}
		dest := flagValue(args[1:], "--output", "")
		path, err := master.BackupDatabase(cfg.DBPath, dest)
		if err != nil {
			return err
		}
		fmt.Println("备份完成：", path)
	case "restore":
		cfg, err := config.LoadMaster(cfgPath)
		if err != nil {
			return err
		}
		src := flagValue(args[1:], "--from", "")
		if src == "" {
			return fmt.Errorf("restore 需要 --from <backup.db>")
		}
		if err := master.RestoreDatabase(src, cfg.DBPath); err != nil {
			return err
		}
		fmt.Println("恢复完成：", cfg.DBPath)
	case "version":
		fmt.Println(version.MasterString())
	default:
		printHelp()
	}
	return nil
}

func printHelp() {
	fmt.Println(`qdr-master commands:
  run [--config path]
  telegram-run [--config path]
  status [--config path]
  config-check [--config path]
  telegram-status [--config path]
  migrate [--config path]
  backup [--config path] [--output path]
  restore --from <backup.db> [--config path]
  version`)
}

func configFlag(args []string, fallback string) (string, bool) {
	for i := 0; i < len(args); i++ {
		if (args[i] == "--config" || args[i] == "--env") && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return fallback, false
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
	out += fmt.Sprintf("DNS 服务商: %s\n", blankAsDash(status.DNSProviderKind))
	out += fmt.Sprintf("%s: %s\n", master.DNSCredentialLabel(status.DNSProviderKind), ternary(status.CloudflareTokenConfigured, "已配置 "+status.CloudflareTokenMasked, "未配置"))
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
