package handler

import (
	"github.com/turbonomic/turbo-go-sdk/pkg/probe"

	"errors"
	"github.com/golang/glog"
	goproto "github.com/golang/protobuf/proto"
	"github.com/turbonomic/turbo-go-sdk/pkg/proto"
)

type MediationMessageProcessorConfig struct {
	probes       map[string]*probe.ProbeProperties
	outgoingChan chan goproto.Message
	incomingChan chan goproto.Message
}

// incomingChan is the encoding layer outgoing channel.
func NewMediationMessageProcessorConfig(probes map[string]*probe.ProbeProperties,
	encodingLayerOutgoingChan chan goproto.Message) *MediationMessageProcessorConfig {
	return &MediationMessageProcessorConfig{
		probes: probes,

		incomingChan: encodingLayerOutgoingChan,
		outgoingChan: make(chan struct{}),
	}
}

type MediationMessageProcessor struct {
	handshake ServerClientHandshake
	config    *MediationMessageProcessorConfig

	pipeline *Pipeline

	stopChan chan struct{}
}

func NewMediationMessageProcessor(config *MediationMessageProcessorConfig) *MediationMessageProcessor {
	return &MediationMessageProcessor{
		handshake: NewServerClientHandshake(config.probes),

		config: config,

		pipeline: initMediationContainerPipeline(),

		stopChan: make(chan struct{}),
	}
}

// Initialize the mediation container communication pipeline.
// For every new connection, the mediation container should first deal with version negotiation and then registration.
// Then it waits and process mediation client message.
func initMediationContainerPipeline() *Pipeline {
	pipeline := NewPipeline()
	pipeline.Push(&negotiationMessageHandler{})
	pipeline.Push(&registrationMessageHandler{})
	pipeline.Push(&mediationServerMessageHandler{})

	return pipeline
}

// Start first initiates the handshake process
func (mmp *MediationMessageProcessor) Start() {

}

func (mmp *MediationMessageProcessor) Reset() {
	if mmp.stopChan == nil {
		mmp.stopChan = make(chan struct{})
	}
	// make sure to stop any listening goroutine.
	mmp.stopChan <- struct{}{}
	mmp.pipeline = initMediationContainerPipeline()

}

func (mmp *MediationMessageProcessor) init() error {
	// 1. start listening goroutine.
	go mmp.listenReceive()

	// 2. finish handshake process.
	for hStatus := mmp.handshake.Status(); hStatus != HandshakeSucceeded; {
		if hStatus != HandshakeProcessing {
			// handshake failed.
			return errors.New("Handshake failed.")
		}
		handshakeMsg, ok := <-mmp.handshake.OutgoingMessage()
		if !ok {
			// outgoing message channel has been closed
			return errors.New("Handshake failed. Channel closed.")
		}
		mmp.sendClientMessage(handshakeMsg)
	}

	return nil
}

func (mmp *MediationMessageProcessor) Stop() {
	close(mmp.stopChan)
	close(mmp.config.outgoingChan)
}

func (mmp *MediationMessageProcessor) sendClientMessage(msg goproto.Message) {
	mmp.config.outgoingChan <- msg
}

func (mmp *MediationMessageProcessor) OutgoingMessage() <-chan goproto.Message {
	return mmp.config.outgoingChan
}

func (mmp *MediationMessageProcessor) listenReceive() {
	glog.V(3).Info("Listening message from server...")
	for {
		select {
		case <-mmp.stopChan:
			// if handshake process has not finished, stop it.
			if mmp.handshake.Status() != HandshakeSucceeded {
				mmp.handshake.Stop()
			}
			glog.V(4).Info("Mediation message processor stops listening on incoming message.")
			return
		case requestContent, ok := <-mmp.config.incomingChan:
			if !ok {
				errors.New("The incoming message channel from encoding layer is closed.")
				break
			}
			mmp.handleReceivedMessage(requestContent)
		}

	}
}

func (mmp *MediationMessageProcessor) handleReceivedMessage(rawMessage []byte) {
	handler, err := mmp.pipeline.Peek()
	if err != nil {
		glog.Errorf("Error handling raw client message: %s", err)
		return
	}
	switch handler.(type) {
	case *negotiationMessageHandler:
		_, err := handler.HandleRawMessage(rawMessage)
		if err != nil {
			glog.Errorf("Negotiation failed: %s", err)
			break
		} else {
			// only handle version negotiation once.
			mmp.pipeline.Pop()
		}
	case *registrationMessageHandler:
		_, err := handler.HandleRawMessage(rawMessage)
		if err != nil {
			glog.Errorf("Registration failed: %s", err)
			break
		} else {
			// only handle registration once.
			mmp.pipeline.Pop()
		}
	case *mediationServerMessageHandler:
		// Handle MediationClientMessages.
		mediationClientMessage, err := handler.HandleRawMessage(rawMessage)
		if err != nil {
			glog.Errorf("%s", err)
		} else {
			msg, ok := mediationClientMessage.(*proto.MediationClientMessage)
			if !ok {
				glog.Errorf("Not a mediation client message: %s", err)
			} else {
				mmp.sendClientMessage(msg)
			}
		}
	}
}
