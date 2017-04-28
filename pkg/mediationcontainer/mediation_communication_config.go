package mediationcontainer

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/golang/glog"
	"github.com/turbonomic/turbo-go-sdk/pkg/mediationcontainer/transport"
)

type ServerMeta struct {
	TurboServer string `json:"turboServer,omitempty"`
}

func (meta *ServerMeta) ValidateServerMeta() error {
	if meta.TurboServer == "" {
		return errors.New("Turbo Server URL is missing")
	}
	if _, err := url.ParseRequestURI(meta.TurboServer); err != nil {
		return fmt.Errorf("Invalid turbo address url: %v", meta)
	}
	return nil
}

type MediationContainerConfig struct {
	ServerMeta
	transport.TransportConfig
}

// Validate the mediation container config and set default value if necessary.
func (containerConfig *MediationContainerConfig) ValidateMediationContainerConfig() error {
	if err := containerConfig.ValidateServerMeta(); err != nil {
		return err
	}
	if err := containerConfig.ValidateTransportConfig(); err != nil {
		return err
	}
	glog.V(4).Infof("The mediation container config is %v", containerConfig)
	return nil
}
