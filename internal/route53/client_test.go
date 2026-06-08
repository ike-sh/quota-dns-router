package route53

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	r53 "github.com/aws/aws-sdk-go-v2/service/route53"
)

func TestListZonesWithMockAWS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "hostedzone") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z123456</Id>
      <Name>example.com.</Name>
      <CallerReference>ref</CallerReference>
      <ResourceRecordSetCount>2</ResourceRecordSetCount>
    </HostedZone>
  </HostedZones>
  <IsTruncated>false</IsTruncated>
  <MaxItems>100</MaxItems>
</ListHostedZonesResponse>`))
	}))
	defer srv.Close()

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	svc := r53.NewFromConfig(cfg, func(o *r53.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
	client := &Client{svc: svc}

	zones, err := client.ListZones(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 1 || zones[0].Name != "example.com" || zones[0].ID != "Z123456" {
		t.Fatalf("unexpected zones %+v", zones)
	}
}
