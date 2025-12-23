package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/chromedp/chromedp"
)

const (
	spotifyURL      = "https://open.spotify.com"
	spotifyTokenURL = spotifyURL + "/api/token"
)

var (
	addr       = os.Getenv("SPOTIFY_TOKENER_ADDR")
	chromePath = os.Getenv("SPOTIFY_TOKENER_CHROME_PATH")
	logLevel   = os.Getenv("SPOTIFY_TOKENER_LOG_LEVEL")
)

func main() {
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	if logLevel != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(logLevel)); err != nil {
			slog.Error("Invalid log level", slog.String("level", logLevel), slog.Any("err", err))
			return
		}

		slog.SetLogLoggerLevel(level)
		slog.Info("Log level set", slog.String("level", logLevel))
	}

	execAllocatorOptions := chromedp.DefaultExecAllocatorOptions[:]
	if chromePath != "" {
		execAllocatorOptions = append(execAllocatorOptions, chromedp.ExecPath(chromePath))
	}
	execAllocatorOptions = append(execAllocatorOptions, chromedp.UserAgent("Mozilla/5.0 (Linux; Android 6.0; Nexus 5 Build/MRA58N) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Mobile Safari/537.36"))
	// execAllocatorOptions = append(execAllocatorOptions, chromedp.Flag("headless", false))

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), execAllocatorOptions...)
	defer allocCancel()
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(slog.Debug))
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(spotifyURL)); err != nil {
		slog.Error("Failed to run chromedp", slog.Any("err", err))
	}

	mux := http.NewServeMux()

	s := &server{
		ctx: ctx,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}

	mux.HandleFunc("/api/token", s.handleToken)

	go s.Start()
	defer s.Stop()

	slog.Info("Server started", slog.String("address", s.server.Addr))
	defer slog.Info("Server stopped")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, os.Kill)
	<-sig
}

type server struct {
	ctx    context.Context
	server *http.Server
}

func (s *server) Start() {
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Failed to start server", slog.Any("err", err))
	}
}

func (s *server) Stop() {
	if err := s.server.Close(); err != nil {
		slog.Error("Failed to stop server", slog.Any("err", err))
	}
}
