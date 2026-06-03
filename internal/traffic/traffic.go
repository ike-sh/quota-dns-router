package traffic

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Snapshot struct {
	Iface string `json:"iface"`
	RX    int64  `json:"rx"`
	TX    int64  `json:"tx"`
}

type State struct {
	Last Snapshot  `json:"last"`
	At   time.Time `json:"at"`
}

type Sample struct {
	Iface        string
	RXBytesTotal int64
	TXBytesTotal int64
	RXDelta      int64
	TXDelta      int64
}

func ReadProcNetDev(path string) (map[string]Snapshot, error) {
	if path == "" {
		path = "/proc/net/dev"
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	out := make(map[string]Snapshot)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo <= 2 {
			continue
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			return nil, fmt.Errorf("无法解析 /proc/net/dev: %s", line)
		}
		rx, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			return nil, err
		}
		tx, err := strconv.ParseInt(fields[8], 10, 64)
		if err != nil {
			return nil, err
		}
		out[iface] = Snapshot{Iface: iface, RX: rx, TX: tx}
	}
	return out, scanner.Err()
}

func DetectDefaultInterface(path string) (string, error) {
	if path == "" {
		path = "/proc/net/route"
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if lineNo == 1 {
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 11 {
			continue
		}
		if fields[1] == "00000000" {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 {
			return iface.Name, nil
		}
	}
	return "", fmt.Errorf("未找到默认出口网卡")
}

func LoadState(path string) (State, error) {
	var st State
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

func SaveState(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func BuildSample(current Snapshot, previous State) Sample {
	sample := Sample{
		Iface:        current.Iface,
		RXBytesTotal: current.RX,
		TXBytesTotal: current.TX,
	}
	if previous.Last.Iface == current.Iface {
		sample.RXDelta = computeDelta(previous.Last.RX, current.RX)
		sample.TXDelta = computeDelta(previous.Last.TX, current.TX)
	}
	return sample
}

func computeDelta(prev, cur int64) int64 {
	if cur >= prev {
		return cur - prev
	}
	return cur
}

func DiscoverPublicIP(override string, client *http.Client) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequest(http.MethodGet, "https://api.ipify.org", nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := ioReadAll(resp)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func ioReadAll(resp *http.Response) ([]byte, error) {
	return io.ReadAll(resp.Body)
}
