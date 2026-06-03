package master

import (
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

func ValidateNodeConfig(node db.Node) error {
	if strings.TrimSpace(node.Name) == "" {
		return fmt.Errorf("节点名称不能为空")
	}
	if strings.TrimSpace(node.GroupID) == "" {
		return fmt.Errorf("节点必须属于一个分组")
	}
	if ip := net.ParseIP(strings.TrimSpace(node.PublicIP)); ip == nil {
		return fmt.Errorf("节点公网 IP 无效")
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
