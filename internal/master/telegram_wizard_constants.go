package master

import (
	"quota-dns-router-go/internal/cloudflare"
)

const (
	pendingMasterURL            = "master_url"
	pendingCloudflareToken      = "cloudflare_token"
	pendingCloudflareZoneName   = "cloudflare_zone_name"
	pendingCloudflareZoneSelect = "cloudflare_zone_select"
	pendingDNSTypeSelect        = "dns_type_select"
	pendingDNSRecordName        = "dns_record_name"
	pendingDNSTTL               = "dns_ttl"
	pendingDNSFixSelect         = "dns_fix_select"
	pendingGroupName            = "group_name"
	pendingNodeName             = "node_name"
	pendingNodeIP               = "node_ip"
	pendingNodeQuota            = "node_quota"
	pendingNodeThreshold        = "node_threshold"
	pendingNodeModeSelect       = "node_mode_select"
	pendingNodeResetDay         = "node_reset_day"
	pendingNodePriority         = "node_priority"
	pendingNodeTrafficOffset    = "node_traffic_offset"
	pendingNodeConfirm          = "node_confirm"
	pendingPolicyValue          = "policy_value"
	sessionSwitchNotice         = "已切换到新的配置流程。"
	defaultNodePriority         = 10
	defaultDNSRecordTTL         = 60
	defaultDNSRecordProxied     = false
	policyFieldThreshold        = "threshold"
	policyFieldQuota            = "quota"
	policyFieldResetDay         = "reset_day"
	sessionKeyGroupID           = "group_id"
	sessionKeyGroupName         = "group_name"
	sessionKeyNodeID            = "node_id"
	sessionKeyNodeFlow          = "node_flow"
	sessionKeyNodePolicySource  = "node_policy_source"
	sessionKeyNodeName          = "node_name"
	sessionKeyNodeIP            = "node_ip"
	sessionKeyNodeQuota         = "node_quota"
	sessionKeyNodeThreshold     = "node_threshold"
	sessionKeyNodeTrafficMode   = "node_traffic_mode"
	sessionKeyNodeResetDay      = "node_reset_day"
	sessionKeyNodePriority      = "node_priority"
	sessionKeyNodeEditField     = "node_edit_field"
	sessionKeyRecordName        = "record_name"
	sessionKeyRecordType        = "record_type"
	sessionKeyRecordID          = "record_id"
	sessionKeyCurrentIP         = "current_ip"
	sessionKeyZoneID            = "zone_id"
	sessionKeyPolicyField       = "policy_field"
	nodeEditFieldQuota          = "quota"
	nodeEditFieldThreshold      = "threshold"
	nodeEditFieldMode           = "mode"
	nodeEditFieldResetDay       = "reset_day"
	nodeEditFieldPriority       = "priority"
)

type telegramSessionMeta struct {
	Data  map[string]string
	Zones []cloudflare.Zone
}
