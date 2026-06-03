package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.cloudflare.com/client/v4"

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	httpClient HTTPClient
	baseURL    string
}

type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type apiResponse[T any] struct {
	Success bool    `json:"success"`
	Errors  []cfErr `json:"errors"`
	Result  T       `json:"result"`
}

type cfErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewClient(httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

func (c *Client) ListZones(ctx context.Context, token string) ([]Zone, error) {
	v := url.Values{}
	v.Set("per_page", "100")
	body, err := c.get(ctx, token, "/zones?"+v.Encode())
	if err != nil {
		return nil, err
	}
	var resp apiResponse[[]Zone]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Result, nil
}

func (c *Client) LookupZoneID(ctx context.Context, token, zoneName string) (string, error) {
	v := url.Values{}
	v.Set("name", zoneName)
	ref, err := c.get(ctx, token, "/zones?"+v.Encode())
	if err != nil {
		return "", err
	}
	var resp apiResponse[[]Zone]
	if err := json.Unmarshal(ref, &resp); err != nil {
		return "", err
	}
	if len(resp.Result) == 0 {
		return "", fmt.Errorf("未找到 Zone: %s", zoneName)
	}
	return resp.Result[0].ID, nil
}

func (c *Client) LookupDNSRecord(ctx context.Context, token, zoneID, recordName string) (DNSRecord, error) {
	v := url.Values{}
	v.Set("name", recordName)
	v.Set("type", "A")
	body, err := c.get(ctx, token, "/zones/"+zoneID+"/dns_records?"+v.Encode())
	if err != nil {
		return DNSRecord{}, err
	}
	var resp apiResponse[[]DNSRecord]
	if err := json.Unmarshal(body, &resp); err != nil {
		return DNSRecord{}, err
	}
	if len(resp.Result) == 0 {
		return DNSRecord{}, fmt.Errorf("未找到 DNS A 记录: %s", recordName)
	}
	return resp.Result[0], nil
}

func (c *Client) LookupDNSRecordAnyType(ctx context.Context, token, zoneID, recordName string) (DNSRecord, error) {
	v := url.Values{}
	v.Set("name", recordName)
	body, err := c.get(ctx, token, "/zones/"+zoneID+"/dns_records?"+v.Encode())
	if err != nil {
		return DNSRecord{}, err
	}
	var resp apiResponse[[]DNSRecord]
	if err := json.Unmarshal(body, &resp); err != nil {
		return DNSRecord{}, err
	}
	if len(resp.Result) == 0 {
		return DNSRecord{}, fmt.Errorf("未找到 DNS 记录: %s", recordName)
	}
	return resp.Result[0], nil
}

func (c *Client) CreateDNSRecord(ctx context.Context, token, zoneID, recordName, ip string, ttl int, proxied bool) (DNSRecord, error) {
	payload := DNSRecord{
		Type:    "A",
		Name:    recordName,
		Content: ip,
		TTL:     ttl,
		Proxied: proxied,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return DNSRecord{}, err
	}
	respBody, err := c.request(ctx, token, http.MethodPost, "/zones/"+zoneID+"/dns_records", body)
	if err != nil {
		return DNSRecord{}, err
	}
	var resp apiResponse[DNSRecord]
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return DNSRecord{}, err
	}
	if !resp.Success {
		return DNSRecord{}, fmt.Errorf("Cloudflare 创建 DNS A 记录失败")
	}
	return resp.Result, nil
}

func (c *Client) UpdateDNSRecord(ctx context.Context, token, zoneID, recordID, recordName, ip string, ttl int, proxied bool) error {
	payload := DNSRecord{
		Type:    "A",
		Name:    recordName,
		Content: ip,
		TTL:     ttl,
		Proxied: proxied,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	respBody, err := c.request(ctx, token, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+recordID, body)
	if err != nil {
		return err
	}
	var resp apiResponse[DNSRecord]
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("Cloudflare 更新失败")
	}
	if strings.TrimSpace(resp.Result.Content) != strings.TrimSpace(ip) {
		return fmt.Errorf("Cloudflare 返回的 IP 与期望不一致")
	}
	return nil
}

func (c *Client) get(ctx context.Context, token, path string) ([]byte, error) {
	return c.request(ctx, token, http.MethodGet, path, nil)
}

func (c *Client) request(ctx context.Context, token, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Cloudflare API 请求失败: %s", resp.Status)
	}
	var generic struct {
		Success bool    `json:"success"`
		Errors  []cfErr `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &generic); err == nil && !generic.Success && len(generic.Errors) > 0 {
		return nil, fmt.Errorf("Cloudflare API 错误: %s", generic.Errors[0].Message)
	}
	return respBody, nil
}
