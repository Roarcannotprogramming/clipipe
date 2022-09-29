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

type HostClip struct {
	Host string
	Clip []byte
}

type Clients struct {
	Named_chans map[string]chan HostClip
	mu          sync.Mutex
}

var (
	port      = flag.Int("port", 8799, "The server port")
	max_saved = flag.Int("max_saved", 10, "The max number of saved clipboard")
	clients   = Clients{Named_chans: make(map[string]chan HostClip)}
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
		log.Printf("Received: %s", in.Msg)
		clients.mu.Lock()
		for id, hc := range clients.Named_chans {
			if id != in.Id {
				hc <- HostClip{Host: in.Id, Clip: in.Msg}
			}
		}
		clients.mu.Unlock()
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
		clients.mu.Lock()
		if _, exists := clients.Named_chans[in.Id]; !exists {
			clients.Named_chans[in.Id] = make(chan HostClip, *max_saved)
		} else {
			log.Fatalf("Client %s already exists", in.Id)
		}
		clients.mu.Unlock()
		for {
			select {
			case tmp := <-clients.Named_chans[in.Id]:
				log.Printf("%v: Sending: %s", stream, tmp.Clip)
				err := stream.Send(&pb.MsgResponse{
					Id: tmp.Host,
					Status: &pb.Status{
						Code:    pb.Code_OK,
						Message: "done",
					},
					Msg: tmp.Clip,
				})
				if err != nil {
					log.Printf("Error sending: %v", err)
					clients.mu.Lock()
					delete(clients.Named_chans, in.Id)
					clients.mu.Unlock()
					return err
				}
			case <-stream.Context().Done():
				log.Printf("Client disconnected")
				clients.mu.Lock()
				delete(clients.Named_chans, in.Id)
				clients.mu.Unlock()
				return nil
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
