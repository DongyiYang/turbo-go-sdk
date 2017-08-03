package handler

import (
	"fmt"

	"github.com/turbonomic/turbo-go-sdk/pkg/proto"

	"github.com/golang/glog"
	goproto "github.com/golang/protobuf/proto"
)

type registrationMessageHandler struct{}

func (nh *registrationMessageHandler) HandleRawMessage(rawMessage []byte) (goproto.Message, error) {
	// Parse the input stream
	registrationAck := &proto.Ack{}
	err := goproto.Unmarshal(rawMessage, registrationAck)
	if err != nil {
		glog.Error("[RegistrationResponse] unmarshaling error: ", err)
		return nil, fmt.Errorf("[RegistrationResponse] Error unmarshalling transport input stream to protobuf message : %s", err)
	}

	return registrationAck, nil
}
