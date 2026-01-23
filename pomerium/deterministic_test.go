package pomerium

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	configpb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func TestEnsureDeterministicConfigOrder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		cfg  *configpb.Config
		want *configpb.Config
	}{
		{
			name: "sorts routes and certificates",
			cfg: &configpb.Config{
				Routes: []*configpb.Route{
					{Name: proto.String("route-b"), From: "https://b.example.com"},
					{Name: proto.String("route-a"), From: "https://a.example.com"},
				},
				Settings: &configpb.Settings{
					Certificates: []*configpb.Settings_Certificate{
						{Id: "cert-b"},
						{Id: "cert-a"},
					},
				},
			},
			want: &configpb.Config{
				Routes: []*configpb.Route{
					{Name: proto.String("route-a"), From: "https://a.example.com"},
					{Name: proto.String("route-b"), From: "https://b.example.com"},
				},
				Settings: &configpb.Settings{
					Certificates: []*configpb.Settings_Certificate{
						{Id: "cert-a"},
						{Id: "cert-b"},
					},
				},
			},
		},
		{
			name: "sorts routes with identical hosts",
			cfg: &configpb.Config{
				Routes: []*configpb.Route{
					{
						Name: proto.String("route-root"),
						From: "https://a.example.com",
						Path: "/",
					},
					{
						Name: proto.String("route-deep"),
						From: "https://a.example.com",
						Path: "/nested",
					},
				},
			},
			want: &configpb.Config{
				Routes: []*configpb.Route{
					{
						Name: proto.String("route-deep"),
						From: "https://a.example.com",
						Path: "/nested",
					},
					{
						Name: proto.String("route-root"),
						From: "https://a.example.com",
						Path: "/",
					},
				},
			},
		},
		{
			name: "leaves sorted config untouched",
			cfg: &configpb.Config{
				Routes: []*configpb.Route{
					{Name: proto.String("route-a"), From: "https://a.example.com"},
				},
				Settings: &configpb.Settings{
					Certificates: []*configpb.Settings_Certificate{
						{Id: "cert-a"},
					},
				},
			},
			want: &configpb.Config{
				Routes: []*configpb.Route{
					{Name: proto.String("route-a"), From: "https://a.example.com"},
				},
				Settings: &configpb.Settings{
					Certificates: []*configpb.Settings_Certificate{
						{Id: "cert-a"},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := proto.Clone(tc.cfg).(*configpb.Config)
			ensureDeterministicConfigOrder(cfg)

			if diff := cmp.Diff(tc.want, cfg, protocmp.Transform()); diff != "" {
				t.Fatalf("unexpected config (-want +got):\n%s", diff)
			}

			again := proto.Clone(cfg).(*configpb.Config)
			ensureDeterministicConfigOrder(again)

			if diff := cmp.Diff(cfg, again, protocmp.Transform()); diff != "" {
				t.Fatalf("ensureDeterministicConfigOrder not idempotent (-first +second):\n%s", diff)
			}
		})
	}
}
