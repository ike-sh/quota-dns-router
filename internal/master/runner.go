package master

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"quota-dns-router-go/internal/cloudflare"
	"quota-dns-router-go/internal/config"
	"quota-dns-router-go/internal/db"
	"quota-dns-router-go/internal/telegram"
)

type Runtime struct {
	Config config.MasterConfig
	Store  *db.Store
	Bot    *telegram.Bot
	DNS    *cloudflare.Client
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
	if err := store.SavePolicy(ctx, policy); err != nil {
		_ = store.Close()
		return nil, err
	}
	if cfg.SuggestedPublicAPIURL != "" {
		_ = store.SetSetting(ctx, settingSuggestedPublicAPIURL, cfg.SuggestedPublicAPIURL)
	} else if suggested := SuggestedPublicAPIURLFromIP(cfg.DetectedPublicIP); suggested != "" {
		_ = store.SetSetting(ctx, settingSuggestedPublicAPIURL, suggested)
	}
	bot := telegram.NewBotForAdmins(cfg.TelegramToken, cfg.TelegramAdminIDs, nil)
	return &Runtime{
		Config: cfg,
		Store:  store,
		Bot:    bot,
		DNS:    cloudflare.NewClient(nil),
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

	errCh := make(chan error, 3)
	svc := NewService(runtime.Store, runtime.Bot, runtime.DNS)
	go func() {
		errCh <- StartHTTP(ctx, cfg, runtime.Store, svc)
	}()
	go func() {
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
		controller := NewTelegramController(runtime.Bot, runtime.Store, cfg.PublicAPIURL, cfg.TelegramPollTimeout, runtime.DNS)
		errCh <- controller.Run(ctx)
	}()
	go func() {
		ticker := time.NewTicker(cfg.CheckInterval)
		defer ticker.Stop()
		var lastReportPurge time.Time
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
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
	}()
	err = <-errCh
	if err == context.Canceled {
		return nil
	}
	return err
}

func TelegramRun(ctx context.Context, cfg config.MasterConfig) error {
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
	controller := NewTelegramController(runtime.Bot, runtime.Store, cfg.PublicAPIURL, cfg.TelegramPollTimeout, runtime.DNS)
	return controller.Run(ctx)
}
