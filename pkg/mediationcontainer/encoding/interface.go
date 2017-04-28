package encoding

import "github.com/turbonomic/turbo-go-sdk/pkg/mediationcontainer/transport"

// Endpoint to handle communication of a particular ProtoBuf message type with the server
type ProtoBufEndpoint interface {
	GetName() string
	GetTransport() transport.ITransport
	CloseEndpoint()
	Send(messageToSend *EndpointMessage)
	GetMessageHandler() ProtoBufMessage
	MessageReceiver() chan *ParsedMessage
}
