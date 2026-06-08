package master

import "strings"

const settingSuggestedPublicAPIURL = "suggested_public_api_url"

func valueOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	return v
}
