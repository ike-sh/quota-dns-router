package master

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

type Runtime struct {
	Config config.MasterConfig
	Store  *db.Store
	Bot    *telegram.Bot
	DNS    DNSProvider
}

func OpenRuntime(ctx context.Context, cfg config.MasterConfig) (*Runtime, error) {
	store, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	policy, err := store.GetPolicy(ctx)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	if secs := int(cfg.AgentOfflineAfter.Seconds()); secs > 0 {
		policy.AgentOfflineSeconds = secs
	}
	if secs := int(cfg.OfflineNotifyAfter.Seconds()); secs > 0 {
		policy.OfflineNotifySeconds = secs
	}
	if err := store.SavePolicy(ctx, policy); err != nil {
		_ = store.Close()
		return nil, err
	}
	dnsProvider := strings.ToLower(strings.TrimSpace(cfg.DNSProvider))
	if dnsProvider == "" {
		dnsProvider = "cloudflare"
	}
	_ = store.SetSetting(ctx, "dns_provider", dnsProvider)
	if IsRoute53Provider(dnsProvider) {
		token, _, _, _ := store.GetCloudflareDefaults(ctx)
		if strings.TrimSpace(token) == "" {
			_ = store.SaveCloudflareDefaults(ctx, Route53PlaceholderToken, "", "")
		}
	}
	if cfg.SuggestedPublicAPIURL != "" {
		_ = store.SetSetting(ctx, settingSuggestedPublicAPIURL, cfg.SuggestedPublicAPIURL)
	} else if suggested := SuggestedPublicAPIURLFromIP(cfg.DetectedPublicIP); suggested != "" {
		_ = store.SetSetting(ctx, settingSuggestedPublicAPIURL, suggested)
	}
	bot := telegram.NewBotForRoles(cfg.TelegramToken, cfg.TelegramAdminIDs, cfg.TelegramObserverIDs, nil)
	dns, err := NewDNSProvider(cfg.DNSProvider, cfg.AWSRegion)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	return &Runtime{
		Config: cfg,
		Store:  store,
		Bot:    bot,
		DNS:    dns,
	}, nil
}

func (r *Runtime) Close() error {
	return r.Store.Close()
}

func Run(ctx context.Context, cfg config.MasterConfig) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := OpenRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	defer runtime.Close()
	_ = runtime.Bot.SendAdminMessage(ctx, "quota-dns-router Master 已启动。发送 /start 继续初始化配置。")

	svc := NewService(runtime.Store, runtime.Bot, runtime.DNS)
	controller := NewTelegramController(runtime.Bot, runtime.Store, cfg.PublicAPIURL, cfg.TelegramPollTimeout, runtime.DNS, cfg.DNSProvider)

	var wg sync.WaitGroup
	errCh := make(chan error, 3)
	startWorker := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil && err != context.Canceled {
				select {
				case errCh <- fmt.Errorf("%s: %w", name, err):
				default:
				}
			}
		}()
	}
	startWorker("http", func() error { return StartHTTP(ctx, cfg, runtime.Store, svc) })
	startWorker("telegram", func() error {
		if me, err := runtime.Bot.GetMe(ctx); err == nil {
			if me.Username != "" {
				fmt.Fprintf(os.Stdout, "Telegram bot connected: @%s\n", me.Username)
			} else {
				fmt.Fprintf(os.Stdout, "Telegram bot connected: bot_id=%d\n", me.ID)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Telegram getMe failed: %v\n", err)
		}
		fmt.Fprintln(os.Stdout, "Telegram long polling started")
		return controller.Run(ctx)
	})
	startWorker("scheduler", func() error {
		ticker := time.NewTicker(cfg.CheckInterval)
		defer ticker.Stop()
		var lastReportPurge time.Time
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if err := svc.CheckOfflineNodes(ctx); err != nil {
					slog.Error("offline check failed", "error", err)
				}
				if err := svc.EvaluateAndSwitchAll(ctx); err != nil {
					slog.Error("auto switch evaluation failed", "error", err)
				}
				if lastReportPurge.IsZero() || time.Since(lastReportPurge) >= 24*time.Hour {
					cutoff := time.Now().AddDate(0, 0, -cfg.AgentReportRetentionDays)
					if count, err := runtime.Store.PurgeAgentReportsBefore(ctx, cutoff); err != nil {
						slog.Error("agent report purge failed", "error", err)
					} else if count > 0 {
						slog.Info("purged old agent reports", "count", count, "retention_days", cfg.AgentReportRetentionDays)
					}
					lastReportPurge = time.Now()
				}
			}
		}
	})

	err = <-errCh
	stop()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		slog.Warn("master shutdown timeout waiting for workers")
	}
	if err == context.Canceled {
		return nil
	}
	return err
}

func TelegramRun(ctx context.Context, cfg config.MasterConfig) error {
	fmt.Fprintln(os.Stderr, "警告：telegram-run 仅启动 Telegram Bot，不启动 HTTP API、离线检查与自动切换。生产环境请使用 qdr-master run。")
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := OpenRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	defer runtime.Close()
	if me, err := runtime.Bot.GetMe(ctx); err == nil {
		if me.Username != "" {
			fmt.Fprintf(os.Stdout, "Telegram bot connected: @%s\n", me.Username)
		} else {
			fmt.Fprintf(os.Stdout, "Telegram bot connected: bot_id=%d\n", me.ID)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Telegram getMe failed: %v\n", err)
	}
	fmt.Fprintln(os.Stdout, "Telegram long polling started")
	controller := NewTelegramController(runtime.Bot, runtime.Store, cfg.PublicAPIURL, cfg.TelegramPollTimeout, runtime.DNS, cfg.DNSProvider)
	return controller.Run(ctx)
}
