package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

func main() {
	restAddr := flag.String("rest-addr", ":8080", "REST server address")
	grpcAddr := flag.String("grpc-addr", ":50051", "gRPC server address")
	flag.Parse()

	// Start gRPC server in background.
	go func() {
		log.Printf("gRPC server listening on %s", *grpcAddr)
		if err := startGRPCServer(*grpcAddr); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	// REST server in foreground.
	fmt.Printf("REST server listening on %s\n", *restAddr)
	if err := http.ListenAndServe(*restAddr, newRESTMux()); err != nil {
		log.Fatalf("REST server error: %v", err)
	}
}
