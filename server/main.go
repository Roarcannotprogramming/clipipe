package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"

	pb "github.com/Roarcannotprogramming/clipipe/proto"

	"google.golang.org/grpc"
)

var (
	port        = flag.Int("port", 8799, "The server port")
	max_saved   = flag.Int("max_saved", 10, "The max number of saved clipboard")
	saved_clips = make([][]byte, *max_saved)
	current     = 0
	mutex       = &sync.RWMutex{}
)

type server struct {
	pb.UnimplementedClipServer
}

func (s *server) Ping(ctx context.Context, in *pb.PingRequest) (*pb.PingResponse, error) {
	if in.Status.Code == pb.Code_OK && in.Status.Message == "ping" {
		out := &pb.PingResponse{
			Status: &pb.Status{
				Code:    pb.Code_OK,
				Message: "pong",
			},
			Id: in.Id,
		}
		return out, nil
	}
	return nil, fmt.Errorf("invalid ping request")
}

func (s *server) Push(ctx context.Context, in *pb.PushRequest) (*pb.PushResponse, error) {
	if in.Status.Code == pb.Code_OK && in.Status.Message == "push" {
		mutex.Lock()
		current = (current + 1) % *max_saved
		saved_clips[current] = in.Msg
		mutex.Unlock()
		out := &pb.PushResponse{
			Status: &pb.Status{
				Code:    pb.Code_OK,
				Message: "done",
			},
			Id: in.Id,
		}
		return out, nil
	}
	return nil, fmt.Errorf("invalid push request")
}

func (s *server) GetStream(in *pb.ConnRequest, stream pb.Clip_GetStreamServer) error {
	if in.Status.Code == pb.Code_OK && in.Status.Message == "connect" {
		for {
			mutex.RLock()
			tmp_msg := saved_clips[current]
			mutex.RUnlock()
			err := stream.Send(&pb.MsgResponse{
				Status: &pb.Status{
					Code:    pb.Code_OK,
					Message: "done",
				},
				Msg: tmp_msg,
			})
			if err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("invalid get request")
}

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
