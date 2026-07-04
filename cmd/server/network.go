package main

import (
	"net"
	"net/http"
	"time"
)

// getLocalIP returns the first non-loopback IPv4 address of the machine.
// Falls back to "localhost" if no suitable interface is found.
func getLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "localhost"
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if ip := v.IP.To4(); ip != nil {
					return ip.String()
				}
			}
		}
	}
	return "localhost"
}

// getPublicIP tries to detect the public-facing IP by querying ipify.org.
// Returns empty string if detection fails (e.g. no internet, timeout).
func getPublicIP(timeout time.Duration) string {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil || n == 0 || resp.StatusCode != http.StatusOK {
		return ""
	}
	return string(buf[:n])
}
