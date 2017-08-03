package handler

import (
	"errors"
	"fmt"

	"github.com/turbonomic/turbo-go-sdk/pkg/proto"

	goproto "github.com/golang/protobuf/proto"
)
//
//func unmarshalNegotiationMessage(rawMessage []byte) (goproto.Message, error) {
//	clientMessage := &version.NegotiationRequest{}
//	err := goproto.Unmarshal(rawMessage, clientMessage)
//	if err != nil {
//		return nil, fmt.Errorf("Cannot unmarshall: %s", err)
//	}
//
//	return clientMessage, nil
//}

type mediationServerMessageHandler struct{}

func (mch *mediationServerMessageHandler) HandleRawMessage(rawMessage []byte) (goproto.Message, error) {
	msg, err := unmarshalMediationServerMessage(rawMessage)
	if err != nil {
		return nil, err
	}
	clientMessage, ok := msg.(*proto.MediationClientMessage)
	if !ok {
		return nil, errors.New("Not a mediation server message")
	}
	return clientMessage, nil
}

func unmarshalMediationServerMessage(rawMessage []byte) (goproto.Message, error) {
	clientMessage := &proto.MediationServerMessage{}
	err := goproto.Unmarshal(rawMessage, clientMessage)
	if err != nil {
		return nil, fmt.Errorf("Cannot unmarshall: %s", err)
	}

	return clientMessage, nil
}
//
//func marshallServerMessage(serverMessage goproto.Message) ([]byte, error) {
//	marshaled, err := goproto.Marshal(serverMessage)
//	if err != nil {
//		return nil, fmt.Errorf("Error marshalling server message %+v", serverMessage)
//	}
//	return marshaled, nil
//}

