package controlplane

import (
	"context"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	"github.com/hnsx-io/hnsx/server/internal/app"
	domainrepo "github.com/hnsx-io/hnsx/server/internal/domain/repository"
	domainsvc "github.com/hnsx-io/hnsx/server/internal/domain/service"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
	"github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1/v1connect"
)

func TestConnectDomainRegistry(t *testing.T) {
	application := &app.Application{
		DomainService: domainsvc.NewService(domainrepo.NewInMemoryRepository()),
	}
	connectSrv := NewConnectServer(application)
	srv := httptest.NewServer(connectSrv.Handler())
	defer srv.Close()

	client := v1connect.NewDomainRegistryServiceClient(
		srv.Client(),
		srv.URL,
		connect.WithHTTPGet(),
	)

	ds, err := domain.Parse([]byte(`
id: test-domain
version: "1.0.0"
description: "connect test"
harness:
  agents:
    a1:
      id: a1
      provider: noop
      adapter:
        kind: noop
  session:
    mode: single
    agent: a1
`))
	if err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	pbSpec, err := domain.ToProto(ds)
	if err != nil {
		t.Fatalf("to proto: %v", err)
	}

	ctx := context.Background()
	reg, err := client.RegisterDomain(ctx, connect.NewRequest(&pb.RegisterDomainRequest{Spec: pbSpec}))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if reg.Msg.GetDomain().GetId() != "test-domain" {
		t.Fatalf("unexpected id: %s", reg.Msg.GetDomain().GetId())
	}

	got, err := client.GetDomain(ctx, connect.NewRequest(&pb.GetDomainRequest{Domain: &pb.DomainRef{Id: "test-domain"}}))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Msg.GetSpec().GetId() != "test-domain" {
		t.Fatalf("unexpected spec id: %s", got.Msg.GetSpec().GetId())
	}

	list, err := client.ListDomains(ctx, connect.NewRequest(&pb.ListDomainsRequest{}))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Msg.GetDomains()) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(list.Msg.GetDomains()))
	}
}
