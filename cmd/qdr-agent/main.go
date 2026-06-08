package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"quota-dns-router-go/internal/agent"
	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/traffic"
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
	cfgPath, cfgPathProvided := configFlag(args[1:], config.DefaultAgentEnvPath)
	switch cmd {
	case "run":
		loadPath := ""
		if cfgPathProvided {
			loadPath = cfgPath
		}
		cfg, err := config.LoadAgent(loadPath, version.Version)
		if err != nil {
			return err
		}
		return agent.NewRunner(cfg).Run(context.Background())
	case "once":
		loadPath := ""
		if cfgPathProvided {
			loadPath = cfgPath
		}
		cfg, err := config.LoadAgent(loadPath, version.Version)
		if err != nil {
			return err
		}
		return agent.NewRunner(cfg).Once(context.Background())
	case "status":
		cfg, err := config.LoadAgent(cfgPath, version.Version)
		if err != nil {
			return err
		}
		diag := traffic.BuildDiagnostics(cfg.Interface, cfg.StateFile, "", "")
		fmt.Print(formatAgentStatus(cfg, diag))
	case "join":
		code := flagValue(args[1:], "--code", "")
		if code == "" {
			code = flagValue(args[1:], "--join", "")
		}
		masterURL := flagValue(args[1:], "--master", "")
		if code == "" || masterURL == "" {
			return fmt.Errorf("join 需要 --code <code> 和 --master <url>")
		}
		resp, err := agent.Join(context.Background(), masterURL, code, nil)
		if err != nil {
			return err
		}
		iface := flagValue(args[1:], "--iface", "")
		if iface != "" {
			resp.Interface = iface
		}
		env := agent.RenderAgentEnv(resp, "")
		envPath := flagValue(args[1:], "--env", config.DefaultAgentEnvPath)
		if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
			return err
		}
		fmt.Println("Agent 配置已写入：", envPath)
	case "config-check":
		cfg, err := config.LoadAgent(cfgPath, version.Version)
		if err != nil {
			return err
		}
		diag := traffic.BuildDiagnostics(cfg.Interface, cfg.StateFile, "", "")
		fmt.Print(formatAgentConfigCheck(cfg, diag))
	case "version":
		fmt.Println(version.AgentString())
	default:
		printHelp()
	}
	return nil
}

func formatAgentStatus(cfg config.AgentConfig, diag traffic.Diagnostics) string {
	lastAt := "-"
	if !diag.LastState.At.IsZero() {
		lastAt = diag.LastState.At.Format(time.RFC3339)
	}
	out := fmt.Sprintf(`Agent 状态：
Master：%s
节点：%s
统计网卡：%s
默认路由网卡：%s
RX：%d
TX：%d
统计模式：%s
最近上报：%s
`, cfg.MasterAPIURL, cfg.NodeName, valueOrDash(diag.SelectedIface), valueOrDash(diag.RouteIface), diag.Snapshot.RX, diag.Snapshot.TX, trafficModeLabel(cfg.TrafficMode), lastAt)
	if diag.Warning != "" {
		out += diag.Warning + "\n"
	}
	if diag.Error != "" {
		out += "错误：" + diag.Error + "\n"
	}
	return out
}

func formatAgentConfigCheck(cfg config.AgentConfig, diag traffic.Diagnostics) string {
	readable := "不可读取"
	if diag.ProcNetDevReadable {
		readable = "可读取"
	}
	out := fmt.Sprintf(`配置检查通过：%s
/proc/net/dev：%s
默认路由网卡：%s
统计网卡：%s
`, cfg.String(), readable, valueOrDash(diag.RouteIface), valueOrDash(diag.SelectedIface))
	if diag.Warning != "" {
		out += diag.Warning + "\n"
	}
	if diag.Error != "" {
		out += "错误：" + diag.Error + "\n"
	}
	return out
}

func trafficModeLabel(mode string) string {
	switch mode {
	case db.TrafficModeRX:
		return "单向 RX"
	case db.TrafficModeTX:
		return "单向 TX"
	default:
		return "双向 RX+TX"
	}
}

func valueOrDash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func printHelp() {
	fmt.Println(`qdr-agent commands:
  run [--config path]
  once [--config path]
  status [--config path]
  join --code <code> --master <url> [--iface eth0] [--env path]
  config-check [--config path]
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
