package version

const Version = "0.1.0-alpha.3"

func MasterString() string {
	return "quota-dns-router master " + Version
}

func AgentString() string {
	return "quota-dns-router agent " + Version
}
