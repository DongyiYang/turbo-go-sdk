package handler

import (
	"fmt"

	"github.com/turbonomic/turbo-go-sdk/pkg/version"

	goproto "github.com/golang/protobuf/proto"

	"github.com/golang/glog"
)

type negotiationMessageHandler struct{}

func (nh *negotiationMessageHandler) HandleRawMessage(rawMessage []byte) (goproto.Message, error) {
	negotiationAnswer := &version.NegotiationAnswer{}
	err := goproto.Unmarshal(rawMessage, negotiationAnswer)
	if err != nil {
		glog.Errorf("Unmarshaling error when try to get the negotiation answer from server: %s", err)
		return nil, fmt.Errorf("Unmarshaling error when try to get the negotiation answer from server: %s", err)
	}

	result := negotiationAnswer.GetNegotiationResult()
	if result != version.NegotiationAnswer_ACCEPTED {
		return nil, fmt.Errorf("Version negotiation failed in state: %s", result)
	}

	return negotiationAnswer, nil
}
