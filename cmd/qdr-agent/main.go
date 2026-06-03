package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"quota-dns-router-go/internal/agent"
	"quota-dns-router-go/internal/config"
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
		st, err := traffic.LoadState(cfg.StateFile)
		if err != nil {
			return err
		}
		fmt.Printf("配置：%s\n最近计数：iface=%s rx=%d tx=%d at=%s\n", cfg.String(), st.Last.Iface, st.Last.RX, st.Last.TX, st.At.Format(time.RFC3339))
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
		fmt.Println("配置检查通过：", cfg.String())
	case "version":
		fmt.Println(version.AgentString())
	default:
		printHelp()
	}
	return nil
}

func printHelp() {
	fmt.Println(`qdr-agent commands:
  run [--config path]
  once [--config path]
  status [--config path]
  join --code <code> --master <url> [--env path]
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
