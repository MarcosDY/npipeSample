package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
	"github.com/spiffe/go-spiffe/v2/proto/spiffe/workload"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	pipeNameFlag = flag.String("pname", `\\.\pipe\spire-agent\public\api`, "pipe name")
)

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*pipeNameFlag, grpc.WithInsecure(), grpc.WithContextDialer(
		winio.DialPipeContext,
	))
	if err != nil {
		log.Fatalf("failed to dial: %+v", err)
	}
	defer conn.Close()

	log.Println("PID:", os.Getpid())

	client := workload.NewSpiffeWorkloadAPIClient(conn)

	header := metadata.Pairs("workload.spiffe.io", "true")
	ctx := metadata.NewOutgoingContext(context.Background(), header)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)

	defer cancel()

	stream, err := client.FetchX509SVID(ctx, &workload.X509SVIDRequest{})
	if err != nil {
		log.Fatalf("failed to fetch SVID: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		log.Fatalf("failed to call recv: %v", err)
	}
	log.Printf("SPIFFEID: %q", resp.Svids[0].SpiffeId)
}
