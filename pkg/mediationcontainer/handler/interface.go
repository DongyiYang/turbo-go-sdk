package handler

import "github.com/turbonomic/turbo-go-sdk/pkg/proto"

type RequestHandler interface {
	HandleMessage(serverRequest proto.MediationServerMessage, probeMsgChan chan *proto.MediationClientMessage)
}
