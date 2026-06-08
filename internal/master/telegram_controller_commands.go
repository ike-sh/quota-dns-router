package master

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"quota-dns-router-go/internal/db"
)

func (c *TelegramController) handleGroupsCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendGroupsPanel(ctx, chatID, c.replaceSession(chatID))
	}
	if len(parts) >= 4 && parts[1] == "rename" {
		group, err := c.Store.GetGroupByName(ctx, parts[2])
		if err != nil {
			return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请先确认原分组名。", nil)
		}
		if err := ValidateGroupName(parts[3]); err != nil {
			return c.sendMessageOrEdit(ctx, chatID, err.Error(), nil)
		}
		if err := c.Store.UpdateGroupName(ctx, group.ID, parts[3]); err != nil {
			return c.sendMessageOrEdit(ctx, chatID, "分组改名失败："+err.Error(), nil)
		}
		return c.sendGroupDetail(ctx, chatID, group.ID, "✅ 分组名称已更新。\n\n")
	}
	if len(parts) >= 3 && parts[1] == "add" {
		if err := ValidateGroupName(parts[2]); err != nil {
			return c.sendMessageOrEdit(ctx, chatID, err.Error(), nil)
		}
		policy, _ := c.Store.GetPolicy(ctx)
		_, err := c.Store.CreateGroup(ctx, parts[2], policy.DefaultSwitchCooldownSecs)
		if err != nil {
			return err
		}
		return c.sendMessageOrEdit(ctx, chatID, "✅ 分组已创建："+parts[2]+"\n\n下一步：", groupCreatedMenu())
	}
	return c.sendMessageOrEdit(ctx, chatID, groupsHelp(), nil)
}

func (c *TelegramController) handleNodesCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendNodesPanel(ctx, chatID, c.replaceSession(chatID))
	}
	if len(parts) >= 5 && parts[1] == "add" {
		group, err := c.Store.GetGroupByName(ctx, parts[4])
		if err != nil {
			return c.sendMessageOrEdit(ctx, chatID, "分组不存在，请先 /groups add "+parts[4], nil)
		}
		policy, _ := c.Store.GetPolicy(ctx)
		node := db.Node{
			GroupID:               group.ID,
			Name:                  parts[2],
			PublicIP:              parts[3],
			MonthlyQuotaBytes:     policy.DefaultMonthlyQuotaBytes,
			ThresholdPercent:      policy.DefaultThresholdPercent,
			ResetDay:              policy.DefaultResetDay,
			TrafficMode:           policy.DefaultTrafficMode,
			Enabled:               true,
			AutoSwitch:            true,
			Priority:              defaultNodePriority,
			PreferredIface:        "auto",
			ReportIntervalSeconds: policy.AgentReportIntervalSeconds,
		}
		for _, item := range parts[5:] {
			k, v, ok := strings.Cut(item, "=")
			if !ok {
				continue
			}
			switch k {
			case "quota":
				if bytes, err := parseGB(v); err == nil {
					node.MonthlyQuotaBytes = bytes
				}
			case "threshold":
				if n, err := strconv.Atoi(strings.TrimSuffix(v, "%")); err == nil {
					node.ThresholdPercent = n
				}
			case "reset_day":
				if n, err := strconv.Atoi(v); err == nil {
					node.ResetDay = n
				}
			case "mode":
				node.TrafficMode = normalizeMode(v)
			case "priority":
				if n, err := strconv.Atoi(v); err == nil {
					node.Priority = n
				}
			case "iface":
				node.PreferredIface = v
			case "enabled":
				node.Enabled = parseBool(v, true)
			case "auto_switch":
				node.AutoSwitch = parseBool(v, true)
			}
		}
		if err := ValidateNodeConfig(node); err != nil {
			return c.sendMessageOrEdit(ctx, chatID, err.Error(), nil)
		}
		created, err := c.Store.CreateNode(ctx, node)
		if err != nil {
			return err
		}
		return c.sendNodeCreatedSummary(ctx, chatID, created)
	}
	return c.sendMessageOrEdit(ctx, chatID, nodesHelp(), nil)
}

func (c *TelegramController) handleDNSCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) == 1 {
		return c.sendDNSPanel(ctx, chatID, c.replaceSession(chatID))
	}
	if len(parts) >= 4 && parts[1] == "set" {
		group, err := c.Store.GetGroupByName(ctx, parts[2])
		if err != nil {
			return err
		}
		ttl := defaultDNSRecordTTL
		proxied := false
		recordID := ""
		for _, item := range parts[4:] {
			k, v, ok := strings.Cut(item, "=")
			if !ok {
				continue
			}
			switch k {
			case "ttl":
				if strings.EqualFold(v, "auto") {
					ttl = 1
				} else if n, err := strconv.Atoi(v); err == nil {
					ttl = n
				}
			case "proxied":
				proxied = parseBool(v, false)
			case "record_id":
				recordID = v
			}
		}
		msg, matchedNodeID, err := c.configureDNSRecord(ctx, group, parts[3], recordID, ttl, proxied)
		if err != nil {
			return c.sendMessageOrEdit(ctx, chatID, err.Error(), nil)
		}
		return c.sendMessageOrEdit(ctx, chatID, msg, dnsSavedMenu(matchedNodeID))
	}
	return c.sendMessageOrEdit(ctx, chatID, dnsHelp(), nil)
}

func (c *TelegramController) handlePolicyCommand(ctx context.Context, chatID int64, parts []string) error {
	policy, err := c.Store.GetPolicy(ctx)
	if err != nil {
		return err
	}
	if len(parts) == 1 {
		return c.sendPolicyPanel(ctx, chatID, c.replaceSession(chatID))
	}
	if len(parts) < 3 || parts[1] != "set" {
		return c.sendMessageOrEdit(ctx, chatID, policyHelp(), nil)
	}
	for _, item := range parts[2:] {
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		switch k {
		case "threshold":
			if n, err := strconv.Atoi(strings.TrimSuffix(v, "%")); err == nil {
				policy.DefaultThresholdPercent = n
			}
		case "quota":
			if n, err := parseGB(v); err == nil {
				policy.DefaultMonthlyQuotaBytes = n
			}
		case "reset_day":
			if n, err := strconv.Atoi(v); err == nil {
				policy.DefaultResetDay = n
			}
		case "mode":
			policy.DefaultTrafficMode = normalizeMode(v)
		case "offline":
			if n, err := strconv.Atoi(v); err == nil {
				policy.AgentOfflineSeconds = n
			}
		case "auto_switch":
			policy.AutoSwitchEnabled = parseBool(v, true)
		case "notify_only":
			policy.NotifyOnly = parseBool(v, false)
		case "repo":
			policy.RepoInstallURL = v
		}
	}
	if err := c.Store.SavePolicy(ctx, policy); err != nil {
		return err
	}
	return c.sendMessageOrEdit(ctx, chatID, "策略已更新。\n"+db.FormatPolicy(policy), policySavedMenu())
}

func (c *TelegramController) handleAgentCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) >= 3 && parts[1] == "install" {
		node, err := c.Store.GetNodeByName(ctx, parts[2])
		if err != nil {
			return err
		}
		return c.sendAgentInstallCommand(ctx, chatID, node.ID)
	}
	return c.sendAgentPanel(ctx, chatID, c.replaceSession(chatID))
}

func (c *TelegramController) handleSwitchCommand(ctx context.Context, chatID int64, parts []string) error {
	if len(parts) < 3 {
		return c.sendSwitchPanel(ctx, chatID, c.replaceSession(chatID))
	}
	group, err := c.Store.GetGroupByName(ctx, parts[1])
	if err != nil {
		return err
	}
	target, err := c.Store.GetNodeByName(ctx, parts[2])
	if err != nil {
		return err
	}
	decision, err := c.buildManualSwitchDecision(ctx, group.ID, target.ID)
	if err != nil {
		return err
	}
	return c.executeManualSwitch(ctx, chatID, decision)
}

func (c *TelegramController) buildManualSwitchDecision(ctx context.Context, groupID, nodeID string) (SwitchDecision, error) {
	if c.DNS == nil {
		return SwitchDecision{}, errors.New("当前进程未配置 DNS 客户端，无法手动切换。")
	}
	group, err := c.Store.GetGroupByID(ctx, groupID)
	if err != nil {
		return SwitchDecision{}, err
	}
	targetNode, err := c.Store.GetNodeByID(ctx, nodeID)
	if err != nil {
		return SwitchDecision{}, err
	}
	if targetNode.GroupID != group.ID {
		return SwitchDecision{}, errors.New("目标节点不属于该分组。")
	}
	cfg, err := c.Store.GetCloudflareConfigByGroupID(ctx, group.ID)
	if err != nil {
		return SwitchDecision{}, err
	}
	if strings.TrimSpace(cfg.RecordName) == "" {
		return SwitchDecision{}, errors.New("当前分组还没有 DNS A 记录，请先完成 DNS 配置。")
	}
	if strings.TrimSpace(cfg.ZoneID) == "" {
		zoneID, lookupErr := c.DNS.LookupZoneID(ctx, cfg.APIToken, cfg.ZoneName)
		if lookupErr != nil {
			return SwitchDecision{}, lookupErr
		}
		cfg.ZoneID = zoneID
		_ = c.Store.SaveCloudflareDefaults(ctx, cfg.APIToken, cfg.ZoneName, zoneID)
		_, _ = c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, cfg.RecordName, cfg.RecordID, dnsRecordType(cfg, ""), cfg.TTL, cfg.Proxied, cfg.AllowOverride)
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		record, lookupErr := c.DNS.LookupDNSRecord(ctx, cfg.APIToken, cfg.ZoneID, cfg.RecordName)
		if lookupErr != nil {
			return SwitchDecision{}, errors.New("当前 DNS 记录还处于待绑定状态，请先在 DNS 面板绑定到节点。")
		}
		cfg.RecordID = record.ID
		_, _ = c.Store.CreateOrUpdateCloudflareConfig(ctx, group.ID, record.Name, record.ID, dnsRecordType(cfg, record.Type), cfg.TTL, cfg.Proxied, cfg.AllowOverride)
	}
	nodes, err := c.Store.ListNodeUsagesByGroup(ctx, group.ID, time.Now())
	if err != nil {
		return SwitchDecision{}, err
	}
	var target db.NodeUsage
	for _, item := range nodes {
		if item.ID == targetNode.ID {
			target = item
			break
		}
	}
	if target.ID == "" {
		target, err = c.Store.GetNodeUsage(ctx, targetNode, time.Now())
		if err != nil {
			return SwitchDecision{}, err
		}
	}
	current := db.NodeUsage{Node: db.Node{Name: "-", PublicIP: "-"}}
	service := Service{Store: c.Store, DNS: c.DNS, Now: time.Now}
	if len(nodes) > 0 {
		if resolved, resolveErr := service.ResolveCurrentNode(ctx, group, cfg, nodes); resolveErr == nil {
			current = resolved
		}
	}
	return SwitchDecision{
		Group:       group,
		Config:      cfg,
		Current:     current,
		Target:      target,
		TriggerType: db.SwitchTriggerManual,
		Reason:      "手动切换",
		Triggered:   true,
	}, nil
}

func (c *TelegramController) executeManualSwitch(ctx context.Context, chatID int64, decision SwitchDecision) error {
	if decision.Current.ID != "" && decision.Current.ID == decision.Target.ID {
		return c.sendMessageOrEdit(ctx, chatID, manualSwitchAlreadyOnTargetMessage(decision), manualSwitchDoneMenu(decision.Target.ID))
	}
	service := Service{Store: c.Store, DNS: c.DNS, Now: time.Now}
	if err := service.ExecuteSwitch(ctx, decision); err != nil {
		return c.sendMessageOrEdit(ctx, chatID, "手动切换失败："+friendlyCloudflareError(err), manualSwitchDoneMenu(decision.Target.ID))
	}
	return c.sendMessageOrEdit(ctx, chatID, manualSwitchSuccessMessage(decision), manualSwitchDoneMenu(decision.Target.ID))
}
