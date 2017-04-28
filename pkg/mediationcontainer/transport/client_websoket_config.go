package transport

import (
	"fmt"
	"net/url"
)

const (
	defaultWebSocketPath         string = "/vmturbo/remoteMediation"
	defaultWebSocketUser         string = "vmtRemoteMediation"
	defaultWebSocketPwd          string = "vmtRemoteMediation"
	defaultWebSocketLocalAddress string = "http://127.0.0.1"
)

type WebSocketConnectionConfig struct {
	turboServer string
	*WebSocketConfig
}

func CreateWebSocketConnectionConfig(turboServer string, tConfig TransportConfig) (*WebSocketConnectionConfig, error) {
	webSocketConfig, ok := tConfig.(*WebSocketConfig)
	if !ok {
		return nil, fmt.Errorf("Not a WebSocket config: %++v", tConfig)
	}
	_, err := url.ParseRequestURI(webSocketConfig.LocalAddress)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse local URL from %s when create WebSocketConnectionConfig.",
			webSocketConfig.LocalAddress)
	}
	// Change URL scheme from ws to http or wss to https.
	serverURL, err := url.ParseRequestURI(turboServer)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse turboServer URL from %s when create WebSocketConnectionConfig.",
			turboServer)
	}
	switch serverURL.Scheme {
	case "http":
		serverURL.Scheme = "ws"
	case "https":
		serverURL.Scheme = "wss"
	}
	//wsConfig := WebSocketConnectionConfig(*connConfig)
	//wsConfig.TurboServer = serverURL.String()

	return &WebSocketConnectionConfig{
		turboServer:     serverURL.String(),
		WebSocketConfig: webSocketConfig,
	}, nil
}

type WebSocketConfig struct {
	LocalAddress      string `json:"localAddress,omitempty"`
	WebSocketUsername string `json:"websocketUsername,omitempty"`
	WebSocketPassword string `json:"websocketPassword,omitempty"`
	ConnectionRetry   int16  `json:"connectionRetry,omitempty"`
	WebSocketPath     string `json:"websocketPath,omitempty"`
}

// Valid a WebSocket config and set default value if needed.
// Return error if the local address in config is invalid.
func (wsc *WebSocketConfig) ValidateTransportConfig() error {
	if wsc.LocalAddress == "" {
		wsc.LocalAddress = defaultWebSocketLocalAddress
	}
	// Make sure the local address string provided is a valid URL
	if _, err := url.ParseRequestURI(wsc.LocalAddress); err != nil {
		return fmt.Errorf("Invalid local address url found in WebSocket config: %v", wsc)
	}

	if wsc.WebSocketPath == "" {
		wsc.WebSocketPath = defaultWebSocketPath
	}
	if wsc.WebSocketUsername == "" {
		wsc.WebSocketUsername = defaultWebSocketUser
	}
	if wsc.WebSocketPassword == "" {
		wsc.WebSocketPassword = defaultWebSocketPwd
	}
	return nil
}
