package main

import (
	"context"
	"encoding/json"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

func init() {
	// Override the default "proto" codec with JSON so that both client and
	// server communicate using JSON payloads over the gRPC HTTP/2 framing.
	// This isolates the protocol overhead (HTTP/2 vs HTTP/1.1) from
	// serialization differences (protobuf vs JSON).
	encoding.RegisterCodec(jsonCodec{})
}

// jsonCodec implements encoding.Codec using JSON.
type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                       { return "proto" } // replaces default proto codec

// BenchServiceServer is the interface gRPC uses to validate the implementation.
// HandlerType in grpc.ServiceDesc must be a pointer to an interface.
type BenchServiceServer interface {
	echo(ctx context.Context, req *EchoReq) (*EchoResp, error)
	getUser(ctx context.Context, req *UserReq) (*UserResp, error)
	createOrder(ctx context.Context, req *OrderReq) (*OrderResp, error)
}

// benchServer implements BenchServiceServer using the same handlers as REST.
type benchServer struct{}

func (s *benchServer) echo(ctx context.Context, req *EchoReq) (*EchoResp, error) {
	resp := handleEcho(*req)
	return &resp, nil
}

func (s *benchServer) getUser(ctx context.Context, req *UserReq) (*UserResp, error) {
	resp := handleGetUser(*req)
	return &resp, nil
}

func (s *benchServer) createOrder(ctx context.Context, req *OrderReq) (*OrderResp, error) {
	resp := handleCreateOrder(*req)
	return &resp, nil
}

// grpcServiceDesc registers the service without protobuf code generation.
var grpcServiceDesc = grpc.ServiceDesc{
	ServiceName: "bench.BenchService",
	HandlerType: (*BenchServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Echo",
			Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				req := new(EchoReq)
				if err := dec(req); err != nil {
					return nil, err
				}
				if interceptor == nil {
					return srv.(BenchServiceServer).echo(ctx, req)
				}
				info := &grpc.UnaryServerInfo{
					Server:     srv,
					FullMethod: "/bench.BenchService/Echo",
				}
				handler := func(ctx context.Context, req any) (any, error) {
					return srv.(BenchServiceServer).echo(ctx, req.(*EchoReq))
				}
				return interceptor(ctx, req, info, handler)
			},
		},
		{
			MethodName: "GetUser",
			Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				req := new(UserReq)
				if err := dec(req); err != nil {
					return nil, err
				}
				if interceptor == nil {
					return srv.(BenchServiceServer).getUser(ctx, req)
				}
				info := &grpc.UnaryServerInfo{
					Server:     srv,
					FullMethod: "/bench.BenchService/GetUser",
				}
				handler := func(ctx context.Context, req any) (any, error) {
					return srv.(BenchServiceServer).getUser(ctx, req.(*UserReq))
				}
				return interceptor(ctx, req, info, handler)
			},
		},
		{
			MethodName: "CreateOrder",
			Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				req := new(OrderReq)
				if err := dec(req); err != nil {
					return nil, err
				}
				if interceptor == nil {
					return srv.(BenchServiceServer).createOrder(ctx, req)
				}
				info := &grpc.UnaryServerInfo{
					Server:     srv,
					FullMethod: "/bench.BenchService/CreateOrder",
				}
				handler := func(ctx context.Context, req any) (any, error) {
					return srv.(BenchServiceServer).createOrder(ctx, req.(*OrderReq))
				}
				return interceptor(ctx, req, info, handler)
			},
		},
	},
	Streams: []grpc.StreamDesc{},
}

func startGRPCServer(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(grpcMetricsInterceptor()))
	s.RegisterService(&grpcServiceDesc, &benchServer{})
	return s.Serve(lis)
}
