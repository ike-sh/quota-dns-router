package system

const MasterUnit = `[Unit]
Description=Quota DNS Router Master
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=quota-dns-router
Group=quota-dns-router
EnvironmentFile=/etc/quota-dns-router/master.env
ExecStart=/usr/local/bin/qdr-master run
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=/var/lib/quota-dns-router /var/log/quota-dns-router

[Install]
WantedBy=multi-user.target
`

const AgentUnit = `[Unit]
Description=Quota DNS Router Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=quota-dns-router
Group=quota-dns-router
EnvironmentFile=/etc/quota-dns-router/agent.env
ExecStart=/usr/local/bin/qdr-agent run
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=/var/lib/quota-dns-router /var/log/quota-dns-router

[Install]
WantedBy=multi-user.target
`
