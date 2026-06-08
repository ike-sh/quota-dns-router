package route53

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	r53 "github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"

	cf "quota-dns-router-go/internal/cloudflare"
)

type Client struct {
	svc *r53.Client
}

func NewClient(ctx context.Context, region string) (*Client, error) {
	if strings.TrimSpace(region) == "" {
		region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("加载 AWS 配置失败: %w", err)
	}
	return &Client{svc: r53.NewFromConfig(cfg)}, nil
}

func (c *Client) ListZones(ctx context.Context, _ string) ([]cf.Zone, error) {
	out, err := c.svc.ListHostedZones(ctx, &r53.ListHostedZonesInput{})
	if err != nil {
		return nil, err
	}
	zones := make([]cf.Zone, 0, len(out.HostedZones))
	for _, z := range out.HostedZones {
		zones = append(zones, cf.Zone{
			ID:   strings.TrimPrefix(awsString(z.Id), "/hostedzone/"),
			Name: strings.TrimSuffix(awsString(z.Name), "."),
		})
	}
	return zones, nil
}

func (c *Client) LookupZoneID(ctx context.Context, _, zoneName string) (string, error) {
	zones, err := c.ListZones(ctx, "")
	if err != nil {
		return "", err
	}
	want := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zoneName)), ".")
	for _, z := range zones {
		if strings.ToLower(z.Name) == want {
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("未找到 hosted zone: %s", zoneName)
}

func (c *Client) LookupDNSRecord(ctx context.Context, _, zoneID, recordName string) (cf.DNSRecord, error) {
	return c.LookupDNSRecordWithType(ctx, "", zoneID, recordName, "A")
}

func (c *Client) LookupDNSRecordWithType(ctx context.Context, _, zoneID, recordName, recordType string) (cf.DNSRecord, error) {
	rec, err := c.findRecord(ctx, zoneID, recordName, recordType)
	if err != nil {
		return cf.DNSRecord{}, err
	}
	return recordFromSet(rec), nil
}

func (c *Client) LookupDNSRecordAnyType(ctx context.Context, _, zoneID, recordName string) (cf.DNSRecord, error) {
	rec, err := c.findRecord(ctx, zoneID, recordName, "")
	if err != nil {
		return cf.DNSRecord{}, err
	}
	return recordFromSet(rec), nil
}

func (c *Client) CreateDNSRecord(ctx context.Context, _, zoneID, recordName, ip string, ttl int, proxied bool) (cf.DNSRecord, error) {
	return c.CreateDNSRecordWithType(ctx, "", zoneID, recordName, ip, "A", ttl, proxied)
}

func (c *Client) CreateDNSRecordWithType(ctx context.Context, _, zoneID, recordName, ip, recordType string, ttl int, _ bool) (cf.DNSRecord, error) {
	if err := c.upsert(ctx, zoneID, recordName, recordType, ip, ttl, ""); err != nil {
		return cf.DNSRecord{}, err
	}
	return c.LookupDNSRecordWithType(ctx, "", zoneID, recordName, recordType)
}

func (c *Client) UpdateDNSRecord(ctx context.Context, _, zoneID, recordID, recordName, ip string, ttl int, proxied bool) error {
	return c.UpdateDNSRecordWithType(ctx, "", zoneID, recordID, recordName, ip, "A", ttl, proxied)
}

func (c *Client) UpdateDNSRecordWithType(ctx context.Context, _, zoneID, recordID, recordName, ip, recordType string, ttl int, _ bool) error {
	return c.upsert(ctx, zoneID, recordName, recordType, ip, ttl, recordID)
}

func (c *Client) findRecord(ctx context.Context, zoneID, recordName, recordType string) (types.ResourceRecordSet, error) {
	name := fqdn(recordName)
	out, err := c.svc.ListResourceRecordSets(ctx, &r53.ListResourceRecordSetsInput{
		HostedZoneId:    awsStringPtr(zoneID),
		StartRecordName: awsStringPtr(name),
		MaxItems:        awsInt32Ptr(100),
	})
	if err != nil {
		return types.ResourceRecordSet{}, err
	}
	wantType := strings.ToUpper(strings.TrimSpace(recordType))
	for _, set := range out.ResourceRecordSets {
		if !strings.EqualFold(awsString(set.Name), name) {
			continue
		}
		if wantType != "" && !strings.EqualFold(string(set.Type), wantType) {
			continue
		}
		return set, nil
	}
	return types.ResourceRecordSet{}, fmt.Errorf("未找到 DNS 记录: %s", recordName)
}

func splitRoute53SetIdentifier(recordID string) string {
	recordID = strings.TrimSpace(recordID)
	if idx := strings.LastIndex(recordID, "#"); idx >= 0 && idx < len(recordID)-1 {
		return recordID[idx+1:]
	}
	return ""
}

func (c *Client) upsert(ctx context.Context, zoneID, recordName, recordType, value string, ttl int, recordID string) error {
	if ttl <= 0 {
		ttl = 300
	}
	rrs := &types.ResourceRecordSet{
		Name: awsStringPtr(fqdn(recordName)),
		Type: types.RRType(strings.ToUpper(recordType)),
		TTL:  awsInt64Ptr(int64(ttl)),
		ResourceRecords: []types.ResourceRecord{{
			Value: awsStringPtr(value),
		}},
	}
	if setID := splitRoute53SetIdentifier(recordID); setID != "" {
		rrs.SetIdentifier = awsStringPtr(setID)
	}
	_, err := c.svc.ChangeResourceRecordSets(ctx, &r53.ChangeResourceRecordSetsInput{
		HostedZoneId: awsStringPtr(zoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{{
				Action:            types.ChangeActionUpsert,
				ResourceRecordSet: rrs,
			}},
		},
	})
	return err
}

func recordFromSet(set types.ResourceRecordSet) cf.DNSRecord {
	content := ""
	if len(set.ResourceRecords) > 0 {
		content = awsString(set.ResourceRecords[0].Value)
	}
	ttl := 0
	if set.TTL != nil {
		ttl = int(*set.TTL)
	}
	id := strings.TrimSuffix(awsString(set.Name), ".") + "/" + string(set.Type)
	if sid := awsString(set.SetIdentifier); sid != "" {
		id = id + "#" + sid
	}
	return cf.DNSRecord{
		ID:      id,
		Type:    string(set.Type),
		Name:    strings.TrimSuffix(awsString(set.Name), "."),
		Content: content,
		TTL:     ttl,
		Proxied: false,
	}
}

func fqdn(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

func awsString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func awsStringPtr(v string) *string { return &v }

func awsInt32Ptr(v int32) *int32 { return &v }

func awsInt64Ptr(v int64) *int64 { return &v }
