package services

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/theborzet/captcha_service/internal/challenge"
	captcha "github.com/theborzet/captcha_service/internal/grpc/capcha"
	pb "github.com/theborzet/captcha_service/pkg/api/pb/captcha/v1"
)

// Server — основной сервер, объединяющий gRPC и балансер
type Server struct {
	GrpcServer     *grpc.Server
	port           int
	log            *slog.Logger
	lis            net.Listener
	captchaService *captcha.GRPCCaptchaService
	balancerClient *BalancerClient
}

func NewCaptchaServer(
	log *slog.Logger,
	challengeStore *challenge.ChallengeStore,
	instanceID, challengeType, captchaHost, balancerHost string,
	captchaPort, balancerPort int,
) *Server {
	grpcServer := grpc.NewServer()

	captchaService := captcha.NewCaptchaService(challengeStore, log)

	// Регистрируем сервис капчи
	pb.RegisterCaptchaServiceServer(grpcServer, captchaService)

	// Включаем reflection для отладки
	reflection.Register(grpcServer)

	balancerClient := NewBalancerClient(
		instanceID,
		challengeType,
		captchaHost,
		captchaPort,
		balancerHost,
		balancerPort,
		log,
	)

	return &Server{
		GrpcServer:     grpcServer,
		port:           captchaPort,
		log:            log,
		captchaService: captchaService,
		balancerClient: balancerClient,
	}
}

func (s *Server) Start(ctx context.Context) error {
	s.log.Info("Starting server")

	// Создаем ListenConfig с SO_REUSEADDR
	listenConfig := &net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				opErr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}

	// Открываем порт
	addr := s.address()
	lis, err := listenConfig.Listen(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	s.lis = lis

	s.log.Info("gRPC server listening", slog.String("addr", addr))

	// Запускаем сервер (блокирующий вызов)
	go func() {
		if err := s.GrpcServer.Serve(lis); err != nil {
			s.log.Error("gRPC server stopped with error", slog.Any("error", err))
		}
	}()

	// Регистрируемся в балансере
	if err := s.balancerClient.Register(ctx); err != nil {
		s.log.Warn("Failed to register with balancer", slog.Any("error", err))
	}

	s.log.Info("Server started")
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.log.Info("Stopping server")

	// Отправляем STOPPED в балансер
	if err := s.balancerClient.SendStopped(ctx); err != nil {
		s.log.Error("Failed to send STOPPED to balancer", slog.Any("error", err))
	}

	s.GrpcServer.GracefulStop()
	if s.lis != nil {
		s.lis.Close()
	}

	return nil
}

func (s *Server) address() string {
	return ":" + strconv.Itoa(s.port)
}
