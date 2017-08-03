package handler

import (
	"errors"

	"github.com/turbonomic/turbo-go-sdk/pkg/probe"
	"github.com/turbonomic/turbo-go-sdk/pkg/proto"
	"github.com/turbonomic/turbo-go-sdk/pkg/version"

	"github.com/golang/glog"
	goproto "github.com/golang/protobuf/proto"
)

const (
	HandshakeSucceeded  HandshakeStatus = "Succeeded"
	HandshakeFailed     HandshakeStatus = "Failed"
	HandshakeProcessing HandshakeStatus = "Processing"
)

type HandshakeStatus string

// The ServerClientHandshakeHandler contains version negotiation and registration.
type ServerClientHandshake struct {
	status       HandshakeStatus
	outgoingChan chan goproto.Message
	allProbes    map[string]*probe.ProbeProperties

	pipeline *Pipeline

	stopChan chan struct{}
}

func NewServerClientHandshake(probes map[string]*probe.ProbeProperties) *ServerClientHandshake {
	pipeline := NewPipeline()
	pipeline.Push(&negotiationMessageHandler{})
	pipeline.Push(&registrationMessageHandler{})

	return &ServerClientHandshake{
		allProbes: probes,

		pipeline: pipeline,

		outgoingChan: make(chan goproto.Message),
		stopChan:     make(chan struct{}),
	}
}

// Stop the server client handshake process.
func (h *ServerClientHandshake) Stop() {
	close(h.stopChan)
	close(h.outgoingChan)
}

func (h *ServerClientHandshake) Status() HandshakeStatus {
	return h.status
}

func (h *ServerClientHandshake) OutgoingMessage() <-chan goproto.Message {
	return h.outgoingChan
}

// Build the version negotiation message.
func (h *ServerClientHandshake) BuildVersionNegotiationRequest() (goproto.Message, error) {
	versionStr := string(proto.PROTOBUF_VERSION)
	request := &version.NegotiationRequest{
		ProtocolVersion: &versionStr,
	}
	return request, nil
}

// Build the containerInfo message as registration request.
func (h *ServerClientHandshake) BuildRegistrationRequest() (goproto.Message, error) {
	if h.allProbes == nil || len(h.allProbes) {
		return nil, errors.New("Probe information is not set.")
	}
	var probes []*proto.ProbeInfo
	for k, v := range h.allProbes {
		if v == nil {
			continue
		}
		glog.V(2).Infof("SdkClientProtocol] Creating Probe Info for", k)
		//var probeInfo *proto.ProbeInfo
		//var err error
		probeInfo, err := v.Probe.GetProbeInfo()
		if err != nil {
			return nil, err
		}
		probes = append(probes, probeInfo)
	}

	return &proto.ContainerInfo{
		Probes: probes,
	}, nil
}

func (nh *ServerClientHandshake) HandleRawMessage(rawMessage []byte) (goproto.Message, error) {
	handler, err := nh.pipeline.Peek()
	if err != nil {
		glog.Errorf("Error handling raw client message: %s", err)
		return
	}
	switch handler.(type) {
	case *negotiationMessageHandler:
		negotiationAnswer, err := handler.HandleRawMessage(rawMessage)
		if err != nil {
			glog.Errorf("Negotiation failed: %s", negotiationAnswer)
			break
		}
		err = nh.SendServerMessage(negotiationAnswer)
		if err != nil {
			glog.Errorf("Failed to send negotiation response: %s", err)
		} else {
			// only handle version negotiation once.
			mc.pipeline.Pop()
		}
	case *registrationMessageHandler:
		registrationAck, err := handler.HandleRawMessage(rawMessage)
		if err != nil {
			glog.Errorf("Registration failed: %s", err)
			break
		}
		glog.V(2).Info("Send out ACK")
		err = mc.SendServerMessage(registrationAck)
		if err != nil {
			glog.Errorf("Failed to send negotiation response: %s", err)
		} else {
			// only handle registration once.
			mc.pipeline.Pop()
		}
	}
}

// Start the handshake process by sending out version negotiation request and registration request.
func (h *ServerClientHandshake) StartHandshake() {
	h.status = HandshakeProcessing
	if h.stopChan == nil {
		h.stopChan = make(chan struct{})
	}
	if h.outgoingChan == nil {
		h.outgoingChan = make(chan goproto.Message)
	}

	var handshakeMessage []goproto.Message

	versionNegotiationRequest, err := h.BuildVersionNegotiationRequest()
	if err != nil {
		h.status = HandshakeFailed
		glog.Fatalf("Failed to build version negotiation request during handshake process with server: %s", err)
	}
	handshakeMessage = append(versionNegotiationRequest)

	registrationRequest, err := h.BuildRegistrationRequest()
	if err != nil {
		h.status = HandshakeFailed
		glog.Fatalf("Failed to build registration request during handshake process with server: %s", err)
	}
	handshakeMessage = append(handshakeMessage, registrationRequest)

	for i := 0; i < 2; i++ {
		select {
		case <-h.stopChan:
			h.status = HandshakeFailed
			glog.Warningf("Handshake process has been stopped.")
			return
		default:
			h.outgoingChan <- handshakeMessage[i]
		}
	}
}
