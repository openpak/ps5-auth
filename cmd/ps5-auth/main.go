// Command ps5-auth serves the OAuth2 surface of ca.account.sony.com for the PlayStation 5 network.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/openpak/ps5-common/accountcore"
	"github.com/openpak/ps5-common/config"
	"github.com/openpak/ps5-common/httpx"
	"github.com/openpak/ps5-common/oauth"

	"openpak/ps5-auth/internal/auth"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(2)
	}
	lvl := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}[cfg.LogLevel]
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
	if cfg.TokenKey == "" {
		log.Error("PS5_TOKEN_KEY is required (at least 32 bytes)")
		os.Exit(2)
	}
	sg, err := oauth.NewSigner([]byte(cfg.TokenKey))
	if err != nil {
		log.Error("signer", "err", err)
		os.Exit(2)
	}
	var core accountcore.Core
	if cfg.CoreURL != "" {
		if core, err = accountcore.New(cfg.CoreURL, cfg.CoreToken, nil); err != nil {
			log.Error("account core", "err", err)
			os.Exit(2)
		}
	} else if !config.DevMemory(os.Getenv) {
		log.Error("PS5_ACCOUNT_CORE_URL and PS5_ACCOUNT_CORE_TOKEN are required (PS5_DEV_MEMORY=1 only for local development)")
		os.Exit(2)
	} else {
		mem := accountcore.NewMemory()
		// Dev only: PS5_DEV_ACCOUNT=email:password:accountId[:onlineId]
		if p := strings.SplitN(os.Getenv("PS5_DEV_ACCOUNT"), ":", 4); len(p) >= 3 {
			mem.AddAccount(p[0], p[1], p[2], accountcore.StatusActive)
			if len(p) == 4 {
				ctx := context.Background()
				l, err := mem.ReserveLink(ctx, p[2], accountcore.Namespace, p[2], "")
				if err == nil {
					_, err = mem.ActivateLink(ctx, l.LinkID, "")
				}
				if err == nil {
					err = mem.SetLinkPublicCode(ctx, l.LinkID, p[3], "")
				}
				if err != nil {
					log.Error("dev account link", "err", err)
				}
			}
		}
		core = mem
		log.Warn("PS5_DEV_MEMORY=1: in-memory accounts, lost on restart (development only)")
	}
	s := &auth.Server{Signer: sg, Core: core, Log: log}
	srv := &http.Server{Addr: cfg.Addr, Handler: httpx.Logger(log, s.Handler()), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	log.Info("ps5-auth listening", "addr", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
