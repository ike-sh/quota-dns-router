package master

import "strings"

func isMutatingCallback(data string) bool {
	if data == "" {
		return false
	}
	readOnlyExact := map[string]bool{
		"menu": true, "status": true, "help": true, "status_refresh": true,
		"setup": true, "cf": true, "cf_view": true, "dns": true, "dns_status": true,
		"groups": true, "groups_status": true, "nodes": true, "nodes_status": true,
		"policy": true, "agent": true, "switch": true,
	}
	if readOnlyExact[data] {
		return false
	}
	readOnlyPrefixes := []string{
		"dns_view:", "groups_view:", "groups_nodes:", "nodes_view:",
		"switch_group:", "dns_group:", "nodes_group:",
		"nodes_traffic_help:",
	}
	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(data, prefix) {
			return false
		}
	}
	return true
}

func isMutatingCommand(text string) bool {
	switch strings.TrimSpace(strings.ToLower(text)) {
	case "", "/start", "/status", "/help", "/cancel":
		return false
	default:
		return strings.HasPrefix(text, "/")
	}
}
