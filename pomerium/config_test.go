package pomerium_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	v1 "github.com/pomerium/ingress-controller/apis/ingress/v1"
	"github.com/pomerium/ingress-controller/model"
	"github.com/pomerium/ingress-controller/pomerium"
	pb "github.com/pomerium/pomerium/pkg/grpc/config"
)

func TestApplyConfig_DownstreamMTLS(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name   string
		expect *pb.DownstreamMtlsSettings
		mtls   *v1.DownstreamMTLS
	}{
		{"nil", nil, nil},
		{"empty", &pb.DownstreamMtlsSettings{}, &v1.DownstreamMTLS{}},
		{
			"ca",
			&pb.DownstreamMtlsSettings{Ca: proto.String("AQIDBA==")},
			&v1.DownstreamMTLS{CA: []byte{1, 2, 3, 4}},
		},
		{
			"crl",
			&pb.DownstreamMtlsSettings{Crl: proto.String("BQYHCA==")},
			&v1.DownstreamMTLS{CRL: []byte{5, 6, 7, 8}},
		},
		{
			"policy_with_default_deny",
			&pb.DownstreamMtlsSettings{Enforcement: pb.MtlsEnforcementMode_POLICY_WITH_DEFAULT_DENY.Enum()},
			&v1.DownstreamMTLS{Enforcement: proto.String("policy_with_default_deny")},
		},
		{
			"policy",
			&pb.DownstreamMtlsSettings{Enforcement: pb.MtlsEnforcementMode_POLICY.Enum()},
			&v1.DownstreamMTLS{Enforcement: proto.String("policy")},
		},
		{
			"reject_connection",
			&pb.DownstreamMtlsSettings{Enforcement: pb.MtlsEnforcementMode_REJECT_CONNECTION.Enum()},
			&v1.DownstreamMTLS{Enforcement: proto.String("REJECT_CONNECTION")},
		},
		{
			"unknown",
			&pb.DownstreamMtlsSettings{},
			&v1.DownstreamMTLS{Enforcement: proto.String("unknown")},
		},
		{
			"dns",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_DNS, Pattern: "DNS"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{DNS: "DNS"}},
		},
		{
			"email",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_EMAIL, Pattern: "EMAIL"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{Email: "EMAIL"}},
		},
		{
			"ip address",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_IP_ADDRESS, Pattern: "IP_ADDRESS"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{IPAddress: "IP_ADDRESS"}},
		},
		{
			"uri",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_URI, Pattern: "URI"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{URI: "URI"}},
		},
		{
			"user principal name",
			&pb.DownstreamMtlsSettings{MatchSubjectAltNames: []*pb.SANMatcher{{SanType: pb.SANMatcher_USER_PRINCIPAL_NAME, Pattern: "USER_PRINCIPAL_NAME"}}},
			&v1.DownstreamMTLS{MatchSubjectAltNames: &v1.MatchSubjectAltNames{UserPrincipalName: "USER_PRINCIPAL_NAME"}},
		},
		{
			"max verify depth",
			&pb.DownstreamMtlsSettings{MaxVerifyDepth: proto.Uint32(23)},
			&v1.DownstreamMTLS{MaxVerifyDepth: proto.Uint32(23)},
		},
	} {
		src := &model.Config{
			Pomerium: v1.Pomerium{
				Spec: v1.PomeriumSpec{
					DownstreamMTLS: tc.mtls,
				},
			},
		}
		dst := new(pb.Config)
		err := pomerium.ApplyConfig(ctx, dst, src)
		assert.NoError(t, err,
			"should have no error in %s", tc.name)
		assert.Empty(t, cmp.Diff(tc.expect, dst.Settings.DownstreamMtls, protocmp.Transform()),
			"should match in %s", tc.name)
	}
}
