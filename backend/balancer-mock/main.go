package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	pb "github.com/theborzet/captcha_service/pkg/api/pb/balancer/v1"
	"google.golang.org/grpc"
)

type BalancerService struct {
	pb.UnimplementedBalancerServiceServer
}

func (s *BalancerService) RegisterInstance(stream pb.BalancerService_RegisterInstanceServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			log.Printf("Stream closed or error: %v", err)
			return err
		}

		log.Printf("Instance: %s, Event: %v, Host: %s, Port: %d",
			req.InstanceId, req.EventType, req.Host, req.PortNumber)

		switch req.EventType {
		case pb.RegisterInstanceRequest_STOPPED:
			log.Printf("STOPPED from %s", req.InstanceId)
			_ = stream.Send(&pb.RegisterInstanceResponse{
				Status:  pb.RegisterInstanceResponse_SUCCESS,
				Message: "STOPPED received",
			})
			return nil
		default:
			_ = stream.Send(&pb.RegisterInstanceResponse{
				Status:  pb.RegisterInstanceResponse_SUCCESS,
				Message: "OK",
			})
		}
	}
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterBalancerServiceServer(grpcServer, &BalancerService{})

	go func() {
		log.Println("Balancer started on :50051")
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Serve error: %v", err)
		}
	}()

	// Graceful shutdown on Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down...")
	grpcServer.GracefulStop()
	log.Println("Balancer stopped")
}
