package master

import (
	"context"
	"fmt"
	"net"
	"strings"

	"quota-dns-router-go/internal/db"
)

func ValidateGroupName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("分组名称不能为空")
	}
	return nil
}

func GroupDNSRecordType(ctx context.Context, store *db.Store, groupID string) string {
	if store == nil || strings.TrimSpace(groupID) == "" {
		return "A"
	}
	cfg, err := store.GetCloudflareConfigByGroupID(ctx, groupID)
	if err != nil {
		return "A"
	}
	if strings.EqualFold(strings.TrimSpace(cfg.RecordType), "AAAA") {
		return "AAAA"
	}
	return "A"
}

func ValidatePublicIPv4(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("公网 IPv4 不能为空")
	}
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("公网 IPv4 无效")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
		return fmt.Errorf("请填写可公网访问的 IPv4，不能使用私网地址、localhost 或 127.0.0.1")
	}
	return nil
}

func ValidatePublicIPv6(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("公网 IPv6 不能为空")
	}
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("公网 IPv6 无效")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
		return fmt.Errorf("请填写可公网访问的 IPv6，不能使用私网、link-local、localhost 或未指定地址")
	}
	return nil
}

func ValidatePublicIP(value, recordType string) error {
	switch strings.ToUpper(strings.TrimSpace(recordType)) {
	case "AAAA":
		return ValidatePublicIPv6(value)
	case "A":
		return ValidatePublicIPv4(value)
	default:
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("公网 IP 不能为空")
		}
		ip := net.ParseIP(value)
		if ip == nil {
			return fmt.Errorf("公网 IP 无效")
		}
		if ip.To4() != nil {
			return ValidatePublicIPv4(value)
		}
		return ValidatePublicIPv6(value)
	}
}

func nodeIPPrompt(recordType string) string {
	if strings.EqualFold(recordType, "AAAA") {
		return "请发送节点公网 IPv6。\n\n要求：\n- 仅支持 IPv6\n- 不允许私网、link-local、localhost\n\n发送 /cancel 取消。"
	}
	return "请发送节点公网 IPv4。\n\n要求：\n- 仅支持 IPv4\n- 不允许私网 IP、localhost、127.0.0.1\n\n发送 /cancel 取消。"
}

func ValidateNodeConfig(node db.Node, recordType string) error {
	if strings.TrimSpace(node.Name) == "" {
		return fmt.Errorf("节点名称不能为空")
	}
	if strings.TrimSpace(node.GroupID) == "" {
		return fmt.Errorf("节点必须属于一个分组")
	}
	if err := ValidatePublicIP(node.PublicIP, recordType); err != nil {
		return err
	}
	if node.MonthlyQuotaBytes <= 0 {
		return fmt.Errorf("月流量总量必须大于 0")
	}
	if node.ThresholdPercent <= 0 || node.ThresholdPercent > 100 {
		return fmt.Errorf("阈值百分比必须在 1-100 之间")
	}
	if node.ResetDay < 1 || node.ResetDay > 28 {
		return fmt.Errorf("重置日必须在 1-28 之间")
	}
	switch node.TrafficMode {
	case db.TrafficModeRX, db.TrafficModeTX, db.TrafficModeBoth:
	default:
		return fmt.Errorf("统计模式必须是 rx、tx 或 rx+tx")
	}
	if node.Priority < 0 {
		return fmt.Errorf("priority 不能小于 0")
	}
	if strings.TrimSpace(node.PreferredIface) == "" {
		return fmt.Errorf("统计网卡不能为空")
	}
	if node.ReportIntervalSeconds <= 0 {
		return fmt.Errorf("Agent 上报间隔必须大于 0")
	}
	return nil
}
