// Command grpc-smoke calls a service over gRPC to verify the internal
// transport, where the REST gateway will not be involved.
//
// Usage:
//
//	go run ./scripts/grpc-smoke -addr localhost:9081 -tenant demo
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	commonv1 "github.com/ank/e-proc/proto/gen/common/v1"
	vendorv1 "github.com/ank/e-proc/proto/gen/vendors/v1"
)

func main() {
	addr := flag.String("addr", "localhost:9081", "gRPC address")
	tenant := flag.String("tenant", "demo", "tenant id")
	service := flag.String("service", "", "service to call: vendor (default) ")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer func() { _ = conn.Close() }()

	healthClient := healthpb.NewHealthClient(conn)
	health, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		log.Fatalf("health check: %v", err)
	}
	fmt.Printf("gRPC health: %s\n", health.GetStatus())

	selected := *service
	if selected == "" {
		selected = "vendor"
	}
	if selected != "vendor" {
		fmt.Fprintf(os.Stderr, "unknown service %q\n", selected)
		os.Exit(1)
	}

	client := vendorv1.NewVendorServiceClient(conn)

	list, err := client.ListVendors(ctx, &vendorv1.ListVendorsRequest{
		TenantId: *tenant,
		Page:     &commonv1.PageRequest{Page: 1, PerPage: 10},
	})
	if err != nil {
		log.Fatalf("ListVendors: %v", err)
	}
	fmt.Printf("ListVendors: %d vendor(s), total=%d\n", len(list.GetVendors()), list.GetPage().GetTotal())
	for _, v := range list.GetVendors() {
		fmt.Printf("  - %s | %s | %s | %s\n", v.GetId(), v.GetCode(), v.GetName(), v.GetStatus())
	}

	if len(list.GetVendors()) > 0 {
		id := list.GetVendors()[0].GetId()
		got, err := client.GetVendor(ctx, &vendorv1.GetVendorRequest{TenantId: *tenant, Id: id})
		if err != nil {
			log.Fatalf("GetVendor: %v", err)
		}
		fmt.Printf("GetVendor(%s): %s\n", id, got.GetVendor().GetName())

		_, err = client.GetVendor(ctx, &vendorv1.GetVendorRequest{TenantId: "tenant-lain", Id: id})
		fmt.Printf("GetVendor dengan tenant lain (harus ditolak): %v\n", err)
	}
}
