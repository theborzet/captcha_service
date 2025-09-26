package services

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	pb "github.com/theborzet/captcha_service/pkg/api/pb/balancer/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// BalancerClient отвечает за регистрацию сервиса капчи в балансере
// и поддержание активного соединения через heartbeat
type BalancerClient struct {
	instanceID    string
	challengeType string
	host          string
	port          int
	balancerHost  string
	balancerPort  int
	log           *slog.Logger

	conn   *grpc.ClientConn
	stream pb.BalancerService_RegisterInstanceClient
}

func NewBalancerClient(
	instanceID, challengeType string,
	captchaHost string,
	captchaPort int,
	balancerHost string,
	balancerPort int,
	log *slog.Logger,
) *BalancerClient {
	return &BalancerClient{
		instanceID:    instanceID,
		challengeType: challengeType,
		host:          captchaHost,
		port:          captchaPort,
		balancerHost:  balancerHost,
		balancerPort:  balancerPort,
		log:           log,
	}
}

// Register регистрирует инстанс капчи в балансере и запускает heartbeat
func (bc *BalancerClient) Register(ctx context.Context) error {
	bc.log.Info("Registering with balancer",
		slog.String("balancer", bc.balancerAddress()),
		slog.String("captcha_host", bc.host),
		slog.Int("captcha_port", bc.port),
		slog.String("instance_id", bc.instanceID),
	)

	conn, err := grpc.Dial(
		bc.balancerAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		bc.log.Error("Failed to connect to balancer", slog.Any("error", err))
		return err
	}
	// Сохраняем соединение для дальнейшего использования и корректного закрытия
	bc.conn = conn

	client := pb.NewBalancerServiceClient(conn)
	stream, err := client.RegisterInstance(ctx)
	if err != nil {
		bc.log.Error("Failed to create registration stream", slog.Any("error", err))
		return err
	}
	bc.stream = stream

	// Отправляем READY
	err = stream.Send(&pb.RegisterInstanceRequest{
		EventType:     pb.RegisterInstanceRequest_READY,
		InstanceId:    bc.instanceID,
		ChallengeType: bc.challengeType,
		Host:          bc.host,
		PortNumber:    int32(bc.port),
		Timestamp:     time.Now().Unix(),
	})
	if err != nil {
		bc.log.Error("Failed to send READY event", slog.Any("error", err))
		return err
	}

	resp, err := stream.Recv()
	if err != nil {
		bc.log.Error("Failed to receive response", slog.Any("error", err))
		return err
	}
	bc.log.Info("Successfully registered with balancer", slog.String("message", resp.Message))

	go bc.heartbeat(stream)
	go bc.readResponses(stream)

	return nil
}

// heartbeat каждые 10 секунд отправляет READY-событие, чтобы балансер знал, что инстанс жив
func (bc *BalancerClient) heartbeat(stream pb.BalancerService_RegisterInstanceClient) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		err := stream.Send(&pb.RegisterInstanceRequest{
			EventType:     pb.RegisterInstanceRequest_READY,
			InstanceId:    bc.instanceID,
			ChallengeType: bc.challengeType,
			Host:          bc.host,
			PortNumber:    int32(bc.port),
			Timestamp:     time.Now().Unix(),
		})
		if err != nil {
			bc.log.Warn("Heartbeat failed", slog.Any("error", err))
			return
		}
	}
}

// readResponses читает ответы от балансера (например, подтверждения или ошибки).
func (bc *BalancerClient) readResponses(stream pb.BalancerService_RegisterInstanceClient) {
	for {
		resp, err := stream.Recv()
		if err != nil {
			bc.log.Info("Balancer disconnected", slog.Any("error", err))
			return
		}
		bc.log.Debug("Response from balancer", slog.Any("response", resp))
	}
}

// SendStopped отправляет событие STOPPED в балансер при graceful shutdown
func (bc *BalancerClient) SendStopped(ctx context.Context) error {
	bc.log.Info("Sending STOPPED to balancer", slog.String("balancer", bc.balancerAddress()))

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		stopCtx,
		bc.balancerAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		bc.log.Error("Failed to connect to balancer for STOPPED",
			slog.Any("error", err),
			slog.String("balancer_addr", bc.balancerAddress()))
		return err
	}
	defer conn.Close()

	client := pb.NewBalancerServiceClient(conn)
	stream, err := client.RegisterInstance(stopCtx)
	if err != nil {
		bc.log.Error("Failed to create stream for STOPPED",
			slog.Any("error", err),
			slog.String("balancer_addr", bc.balancerAddress()))
		return err
	}

	err = stream.Send(&pb.RegisterInstanceRequest{
		EventType:     pb.RegisterInstanceRequest_STOPPED,
		InstanceId:    bc.instanceID,
		ChallengeType: bc.challengeType,
		Host:          bc.host,
		PortNumber:    int32(bc.port),
		Timestamp:     time.Now().Unix(),
	})
	if err != nil {
		bc.log.Error("Failed to send STOPPED event",
			slog.Any("error", err),
			slog.String("balancer_addr", bc.balancerAddress()))
		return err
	}

	// Получаем подтверждение, но с защитой от зависания
	recvCh := make(chan *pb.RegisterInstanceResponse, 1)
	errCh := make(chan error, 1)

	go func() {
		resp, err := stream.Recv()
		if err != nil {
			errCh <- err
			return
		}
		recvCh <- resp
	}()

	select {
	case <-stopCtx.Done():
		bc.log.Warn("Timeout while waiting for STOPPED response")
		return stopCtx.Err()
	case err := <-errCh:
		bc.log.Error("Failed to receive STOPPED response", slog.Any("error", err))
		return err
	case resp := <-recvCh:
		bc.log.Info("STOPPED successfully sent", slog.String("message", resp.Message))
	}

	return stream.CloseSend()
}

func (bc *BalancerClient) Close(ctx context.Context) error {
	if bc.stream != nil {
		err := bc.stream.CloseSend()
		if err != nil {
			bc.log.Error("Failed to close stream", slog.Any("error", err))
		}
	}

	if bc.conn != nil {
		return bc.conn.Close()
	}

	return nil
}

func (bc *BalancerClient) balancerAddress() string {
	return bc.balancerHost + ":" + strconv.Itoa(bc.balancerPort)
}
