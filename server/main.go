package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	pb "github.com/Roarcannotprogramming/clipipe/proto"

	"google.golang.org/grpc"
)

var (
	port = flag.Int("port", 8799, "The server port")
)

type server struct {
	pb.UnimplementedClipServer
}

func (s *server) Ping(ctx context.Context, in *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{Id: in.Id}, nil
}

// func (s *server)

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	pb.RegisterClipServer(s, &server{})
	log.Printf("Server listening on port %d", *port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
