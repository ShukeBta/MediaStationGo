package service

import "strings"

func fallbackSessionDeviceID(deviceName, client, remoteEndPoint string) string {
	parts := []string{strings.TrimSpace(deviceName), strings.TrimSpace(client), strings.TrimSpace(remoteEndPoint)}
	joined := strings.Trim(strings.Join(parts, "|"), "|")
	if joined == "" {
		joined = "unknown"
	}
	return "rt-" + fingerprint(client, joined)
}

func sessionDeviceKey(sess RealtimeSession) string {
	return realtimeSessionTerminalKey(sess.DeviceID, sess.DeviceName, sess.Client, sess.RemoteEndPoint)
}

func realtimeSessionTerminalKey(deviceID, deviceName, client, remoteEndPoint string) string {
	deviceName = strings.TrimSpace(deviceName)
	if deviceName != "" {
		return "fp-" + fingerprint(client, deviceName)
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID != "" {
		return deviceID
	}
	return fallbackSessionDeviceID(deviceName, client, remoteEndPoint)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
