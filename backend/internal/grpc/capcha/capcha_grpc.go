package captcha

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"

	"github.com/theborzet/captcha_service/internal/challenge"
	pb "github.com/theborzet/captcha_service/pkg/api/pb/captcha/v1"
)

type GRPCCaptchaService struct {
	pb.UnimplementedCaptchaServiceServer
	store *challenge.ChallengeStore
	log   *slog.Logger
}

func NewCaptchaService(store *challenge.ChallengeStore, log *slog.Logger) *GRPCCaptchaService {
	return &GRPCCaptchaService{store: store, log: log}
}

func (s *GRPCCaptchaService) NewChallenge(ctx context.Context, req *pb.ChallengeRequest) (*pb.ChallengeResponse, error) {
	s.log.Info("Received CAPTCHA generation request", slog.Int("complexity", int(req.Complexity)))

	challengeID, html, err := challenge.GenerateDragDropChallenge(s.store, int(req.Complexity))
	if err != nil {
		s.log.Error("Failed to generate CAPTCHA", slog.Any("error", err))
		return nil, err
	}
	s.log.Info("CAPTCHA created", slog.String("challenge_id", challengeID))

	return &pb.ChallengeResponse{
		ChallengeId: challengeID,
		Html:        html,
	}, nil
}

func (s *GRPCCaptchaService) MakeEventStream(stream pb.CaptchaService_MakeEventStreamServer) error {
	s.log.Info("Event stream opened")

	for {
		clientEvent, err := stream.Recv()
		if err != nil {
			s.log.Info("Client disconnected", slog.Any("error", err))
			return err
		}

		switch clientEvent.EventType {
		case pb.ClientEvent_FRONTEND_EVENT:
			if err := s.handleFrontendEvent(stream, clientEvent); err != nil {
				s.log.Error("Failed to handle frontend event", slog.Any("error", err))
				return err
			}
		case pb.ClientEvent_CONNECTION_CLOSED:
			s.log.Info("Client closed connection", slog.String("challenge_id", clientEvent.ChallengeId))
			return nil
		case pb.ClientEvent_BALANCER_EVENT:
			s.log.Info("Balancer event received", slog.String("challenge_id", clientEvent.ChallengeId))
		default:
			s.log.Warn("Unknown event type", slog.Int("event_type", int(clientEvent.EventType)))
		}
	}
}

func (s *GRPCCaptchaService) handleFrontendEvent(stream pb.CaptchaService_MakeEventStreamServer, event *pb.ClientEvent) error {
	var payload struct {
		Event       string `json:"event"`
		X           int    `json:"x"`
		Y           int    `json:"y"`
		Success     bool   `json:"success"`
		ChallengeID string `json:"challenge_id"`
	}

	if err := json.Unmarshal(event.Data, &payload); err != nil {
		s.log.Warn("Failed to parse event data", slog.Any("error", err))
		return stream.Send(&pb.ServerEvent{
			Event: &pb.ServerEvent_Result{
				Result: &pb.ServerEvent_ChallengeResult{
					ChallengeId:       "error: invalid event data",
					ConfidencePercent: 0,
				},
			},
		})
	}

	s.log.Info("Event received",
		slog.String("event", payload.Event),
		slog.String("challenge_id", payload.ChallengeID),
		slog.Int("x", payload.X),
		slog.Int("y", payload.Y),
		slog.Bool("success", payload.Success))

	answer, expectedX, expectedY, exists := s.store.Get(payload.ChallengeID)
	if !exists {
		s.log.Warn("CAPTCHA not found or expired", slog.String("challenge_id", payload.ChallengeID))
		return stream.Send(&pb.ServerEvent{
			Event: &pb.ServerEvent_Result{
				Result: &pb.ServerEvent_ChallengeResult{
					ChallengeId:       "error: CAPTCHA not found or expired",
					ConfidencePercent: 0,
				},
			},
		})
	}

	maxDistance := 50.0 / float64(s.store.GetComplexity(payload.ChallengeID))
	distance := math.Sqrt(math.Pow(float64(payload.X-expectedX), 2) + math.Pow(float64(payload.Y-expectedY), 2))
	confidence := 0
	if distance <= maxDistance && payload.Success && answer == "success" {
		confidence = 100 - int(distance*100/maxDistance)
	}

	result := &pb.ServerEvent{
		Event: &pb.ServerEvent_Result{
			Result: &pb.ServerEvent_ChallengeResult{
				ChallengeId:       payload.ChallengeID,
				ConfidencePercent: int32(confidence),
			},
		},
	}

	if err := stream.Send(result); err != nil {
		s.log.Error("Failed to send result", slog.Any("error", err))
		return err
	}

	s.log.Info("Result sent",
		slog.String("challenge_id", payload.ChallengeID),
		slog.Int("confidence", confidence),
		slog.Float64("distance", distance))

	s.store.Delete(payload.ChallengeID)
	return nil
}
