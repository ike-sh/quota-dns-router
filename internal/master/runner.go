package master

import (
	"context"
	"fmt"
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
	bot := telegram.NewBot(cfg.TelegramToken, cfg.TelegramAdminID, nil)
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
	go func() {
		errCh <- StartHTTP(ctx, cfg, runtime.Store)
	}()
	go func() {
		controller := NewTelegramController(runtime.Bot, runtime.Store, cfg.PublicAPIURL, cfg.TelegramPollTimeout, runtime.DNS)
		errCh <- controller.Run(ctx)
	}()
	go func() {
		svc := NewService(runtime.Store, runtime.Bot, runtime.DNS)
		ticker := time.NewTicker(cfg.CheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case <-ticker.C:
				if err := svc.EvaluateAndSwitchAll(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "自动检查失败：%v\n", err)
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
	controller := NewTelegramController(runtime.Bot, runtime.Store, cfg.PublicAPIURL, cfg.TelegramPollTimeout, runtime.DNS)
	return controller.Run(ctx)
}
