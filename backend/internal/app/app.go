package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/theborzet/captcha_service/internal/challenge"
	"github.com/theborzet/captcha_service/internal/config"
	"github.com/theborzet/captcha_service/internal/services"
	"github.com/theborzet/captcha_service/internal/websocket"
	pb "github.com/theborzet/captcha_service/pkg/api/pb/captcha/v1"
	"github.com/theborzet/captcha_service/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type App struct {
	Server      *services.Server
	log         *slog.Logger
	cfg         *config.Config
	captchaPort int
}

func New(log *slog.Logger, cfg *config.Config) *App {
	challengeStore := challenge.NewInMemoryStore()
	captchaPort := utils.FindAvailablePort(cfg.Server.MinPort, cfg.Server.MaxPort)
	serverApp := services.NewCaptchaServer(
		log,
		challengeStore,
		cfg.Instance.ID,
		cfg.Instance.ChallengeType,
		cfg.Host,
		cfg.Balancer.Host,
		captchaPort,
		cfg.Balancer.Port,
	)
	return &App{Server: serverApp, log: log, cfg: cfg, captchaPort: captchaPort}
}

func (a *App) Run(ctx context.Context) error {
	// Канал для обработки сигналов
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := a.Server.Start(ctx); err != nil {
			a.log.Error("Ошибка запуска сервера", slog.Any("error", err))
		}
	}()

	// Подключаемся к gRPC-серверу с ретраями
	var conn *grpc.ClientConn
	for i := 0; i < 10; i++ {
		var err error
		conn, err = grpc.Dial(
			fmt.Sprintf("%s:%d", a.cfg.Host, a.captchaPort),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(time.Second),
		)
		if err == nil {
			break
		}
		a.log.Warn("Waiting for gRPC server", slog.Any("attempt", i+1), slog.Any("error", err))
		time.Sleep(500 * time.Millisecond)
	}
	if conn == nil {
		err := fmt.Errorf("failed to connect to gRPC server after retries")
		a.log.Error("Couldn't connect to gRPC server", slog.Any("error", err))
		return err
	}
	defer conn.Close()

	client := pb.NewCaptchaServiceClient(conn)

	// Настраиваем HTTP-сервер
	srv := &http.Server{Addr: ":8080"}
	http.Handle("/ws", websocket.NewProxy(client, a.log, ctx))
	http.HandleFunc("/captcha", func(w http.ResponseWriter, r *http.Request) {
		complexity := 1
		if compStr := r.URL.Query().Get("complexity"); compStr != "" {
			if comp, err := strconv.Atoi(compStr); err == nil && comp > 0 && comp <= 3 {
				complexity = comp
			}
		}
		resp, err := client.NewChallenge(ctx, &pb.ChallengeRequest{Complexity: int32(complexity)})
		if err != nil {
			a.log.Error("Failed to generate CAPTCHA", slog.Any("error", err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		a.log.Info("Returning CAPTCHA HTML", slog.String("html", resp.Html)) // Отладка
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, err = w.Write([]byte(resp.Html))
		if err != nil {
			a.log.Error("Failed to write CAPTCHA HTML", slog.Any("error", err))
		}
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "frontend/public/drag-drop/index.html")
	})

	a.log.Info("HTTP server with WebSocket proxy running on :8080")
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.log.Error("HTTP server stopped", slog.Any("error", err))
		}
	}()

	a.log.Info("Application started")
	<-sigCh

	// Graceful shutdown
	a.log.Info("Received shutdown signal")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		a.log.Error("HTTP server shutdown failed", slog.Any("error", err))
	}
	if err := a.Server.Stop(shutdownCtx); err != nil {
		a.log.Error("Ошибка остановки сервера", slog.Any("error", err))
	}

	a.log.Info("Server stopped")
	return nil

}
