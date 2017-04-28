package handler

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/turbonomic/turbo-go-sdk/pkg/probe"
	"github.com/turbonomic/turbo-go-sdk/pkg/proto"
	"time"
)

type RequestType string

const (
	DISCOVERY_REQUEST  RequestType = "Discovery"
	VALIDATION_REQUEST RequestType = "Validation"
	INTERRUPT_REQUEST  RequestType = "Interrupt"
	ACTION_REQUEST     RequestType = "Action"
	UNKNOWN_REQUEST    RequestType = "Unknown"
)

func getRequestType(serverRequest proto.MediationServerMessage) RequestType {
	if serverRequest.GetValidationRequest() != nil {
		return VALIDATION_REQUEST
	} else if serverRequest.GetDiscoveryRequest() != nil {
		return DISCOVERY_REQUEST
	} else if serverRequest.GetActionRequest() != nil {
		return ACTION_REQUEST
	} else if serverRequest.GetInterruptOperation() > 0 {
		return INTERRUPT_REQUEST
	} else {
		return UNKNOWN_REQUEST
	}
}

type ServerMessageHandler struct {
	messageHandlers map[RequestType]RequestHandler
}

func NewServerMessageHandler(allProbes map[string]*probe.ProbeProperties) *ServerMessageHandler {
	supportedMessageHandlers := make(map[RequestType]RequestHandler)

	supportedMessageHandlers[DISCOVERY_REQUEST] = &DiscoveryRequestHandler{
		probes: allProbes,
	}
	supportedMessageHandlers[VALIDATION_REQUEST] = &ValidationRequestHandler{
		probes: allProbes,
	}
	supportedMessageHandlers[INTERRUPT_REQUEST] = &InterruptMessageHandler{
		probes: allProbes,
	}
	supportedMessageHandlers[ACTION_REQUEST] = &ActionMessageHandler{
		probes: allProbes,
	}

	var keys []RequestType
	for k := range supportedMessageHandlers {
		keys = append(keys, k)
	}
	glog.V(4).Infof("Created message handlers for server message types : [%s]", keys)

	return &ServerMessageHandler{
		messageHandlers: supportedMessageHandlers,
	}
}

func (handler *ServerMessageHandler) HandleMessage(serverRequest proto.MediationServerMessage,
	probeMsgChan chan *proto.MediationClientMessage) {

	requestType := getRequestType(serverRequest)
	requestHandler, err := handler.getHandler(requestType)
	if err != nil {
		glog.Errorf("[ServerMessageHandler] Error: %s", err)
	}
	// Dispatch on a new thread
	// TODO: create MessageOperationRunner to handle this request for a specific message id
	go requestHandler.HandleMessage(serverRequest, probeMsgChan)
	glog.V(3).Infof("[ServerMessageHandler] %s request message dispatched, waiting for next one", requestType)
}

// Find request handler according to request type.
func (handler *ServerMessageHandler) getHandler(rType RequestType) (RequestHandler, error) {
	requestHandler, exist := handler.messageHandlers[rType]
	if !exist {
		return nil, fmt.Errorf("Cannot find message handler for request type %s",
			string(rType))
	}
	return requestHandler, nil
}

// -------------------------------- Discovery Request Handler -----------------------------------
type DiscoveryRequestHandler struct {
	probes map[string]*probe.ProbeProperties
}

func (discReqHandler *DiscoveryRequestHandler) HandleMessage(serverRequest proto.MediationServerMessage,
	probeMsgChan chan *proto.MediationClientMessage) {
	request := serverRequest.GetDiscoveryRequest()
	probeType := request.ProbeType

	probeProps, exist := discReqHandler.probes[*probeType]
	if !exist {
		glog.Errorf("Received: discovery request for unknown probe type: %s", *probeType)
		return
	}
	glog.V(3).Infof("Received: discovery for probe type: %s", *probeType)

	turboProbe := probeProps.Probe
	msgID := serverRequest.GetMessageID()

	stopCh := make(chan struct{})
	defer close(stopCh)
	go func() {
		for {
			discReqHandler.keepDiscoveryAlive(msgID, probeMsgChan)

			t := time.NewTimer(time.Second * 10)
			select {
			case <-stopCh:
				glog.V(4).Infof("Cancel keep alive for msgID ", msgID)
				return
			case <-t.C:
			}
		}

	}()

	var discoveryResponse *proto.DiscoveryResponse
	discoveryResponse = turboProbe.DiscoverTarget(request.GetAccountValue())
	clientMsg := NewClientMessageBuilder(msgID).SetDiscoveryResponse(discoveryResponse).Create()

	// Send the response on the callback channel to send to the server
	probeMsgChan <- clientMsg // This will block till the channel is ready to receive
	glog.V(3).Infof("Sent discovery response for %d", clientMsg.GetMessageID())

	// Send empty response to signal completion of discovery
	discoveryResponse = &proto.DiscoveryResponse{}
	clientMsg = NewClientMessageBuilder(msgID).SetDiscoveryResponse(discoveryResponse).Create()

	probeMsgChan <- clientMsg // This will block till the channel is ready to receive
	glog.V(3).Infof("Discovery has finished for %d", clientMsg.GetMessageID())

	// Cancel keep alive
	// Note  : Keep alive routine is cancelled when the stopCh is closed at the end of this method
	// when the discovery response is out on the probeMsgCha
}

// Send the KeepAlive message to server in order to inform server the discovery is stil ongoing. Prevent timeout.
func (discReqHandler *DiscoveryRequestHandler) keepDiscoveryAlive(msgID int32, probeMsgChan chan *proto.MediationClientMessage) {
	keepAliveMsg := new(proto.KeepAlive)
	clientMsg := NewClientMessageBuilder(msgID).SetKeepAlive(keepAliveMsg).Create()

	// Send the response on the callback channel to send to the server
	probeMsgChan <- clientMsg // This will block till the channel is ready to receive
	glog.V(3).Infof("Sent keep alive response ", clientMsg.GetMessageID())
}

// -------------------------------- Validation Request Handler -----------------------------------
type ValidationRequestHandler struct {
	probes map[string]*probe.ProbeProperties //TODO: synchronize access to the probes map
}

func (valReqHandler *ValidationRequestHandler) HandleMessage(serverRequest proto.MediationServerMessage,
	probeMsgChan chan *proto.MediationClientMessage) {
	request := serverRequest.GetValidationRequest()
	probeType := request.ProbeType
	probeProps, exist := valReqHandler.probes[*probeType]
	if !exist {
		glog.Errorf("Received: validation request for unknown probe type : %s", *probeType)
		return
	}
	glog.V(3).Infof("Received: validation for probe type: %s\n ", *probeType)
	turboProbe := probeProps.Probe

	var validationResponse *proto.ValidationResponse
	validationResponse = turboProbe.ValidateTarget(request.GetAccountValue())

	msgID := serverRequest.GetMessageID()
	clientMsg := NewClientMessageBuilder(msgID).SetValidationResponse(validationResponse).Create()

	// Send the response on the callback channel to send to the server
	probeMsgChan <- clientMsg // This will block till the channel is ready to receive
	glog.V(3).Infof("Sent validation response ", clientMsg.GetMessageID())
}

// -------------------------------- Action Request Handler -----------------------------------
// Message handler that will receive the Action Request for entities in the TurboProbe.
// Action request will be delegated to the right TurboProbe. Multiple ActionProgress and final ActionResult
// responses are sent back to the server.
type ActionMessageHandler struct {
	probes map[string]*probe.ProbeProperties
}

func (actionReqHandler *ActionMessageHandler) HandleMessage(serverRequest proto.MediationServerMessage,
	probeMsgChan chan *proto.MediationClientMessage) {
	glog.V(4).Infof("[ActionMessageHandler] Received: action %s request", serverRequest)
	request := serverRequest.GetActionRequest()
	probeType := request.ProbeType
	if actionReqHandler.probes[*probeType] == nil {
		glog.Errorf("Received: Action request for unknown probe type : ", *probeType)
		return
	}

	glog.V(3).Infof("Received: action %s request for probe type: %s\n ",
		request.ActionExecutionDTO.ActionType, *probeType)
	probeProps := actionReqHandler.probes[*probeType]
	turboProbe := probeProps.Probe

	msgID := serverRequest.GetMessageID()
	worker := NewActionResponseWorker(msgID, turboProbe,
		request.ActionExecutionDTO, request.GetAccountValue(), probeMsgChan)
	worker.start()
}

// Worker Object that will receive multiple action progress responses from the TurboProbe
// before the final result. Action progress and result are sent to the server as responses for the action request.
// It implements the ActionProgressTracker interface.
type ActionResponseWorker struct {
	msgId              int32
	turboProbe         *probe.TurboProbe
	actionExecutionDto *proto.ActionExecutionDTO
	accountValues      []*proto.AccountValue
	probeMsgChan       chan *proto.MediationClientMessage
}

func NewActionResponseWorker(msgId int32, turboProbe *probe.TurboProbe,
	actionExecutionDto *proto.ActionExecutionDTO, accountValues []*proto.AccountValue,
	probeMsgChan chan *proto.MediationClientMessage) *ActionResponseWorker {
	worker := &ActionResponseWorker{
		msgId:              msgId,
		turboProbe:         turboProbe,
		actionExecutionDto: actionExecutionDto,
		accountValues:      accountValues,
		probeMsgChan:       probeMsgChan,
	}
	glog.V(4).Infof("New ActionResponseProtocolWorker for %s %s %s", msgId, turboProbe,
		actionExecutionDto.ActionType)
	return worker
}

func (actionWorker *ActionResponseWorker) start() {
	var actionResult *proto.ActionResult
	// Execute the action
	actionResult = actionWorker.turboProbe.ExecuteAction(actionWorker.actionExecutionDto, actionWorker.accountValues, actionWorker)
	clientMsg := NewClientMessageBuilder(actionWorker.msgId).SetActionResponse(actionResult).Create()

	// Send the response on the callback channel to send to the server
	actionWorker.probeMsgChan <- clientMsg // This will block till the channel is ready to receive
	glog.V(3).Infof("Sent action response for %d.", clientMsg.GetMessageID())
}

func (actionWorker *ActionResponseWorker) UpdateProgress(actionState proto.ActionResponseState,
	description string, progress int32) {
	// Build ActionProgress
	actionResponse := &proto.ActionResponse{
		ActionResponseState: &actionState,
		ResponseDescription: &description,
		Progress:            &progress,
	}

	actionProgress := &proto.ActionProgress{
		Response: actionResponse,
	}

	clientMsg := NewClientMessageBuilder(actionWorker.msgId).SetActionProgress(actionProgress).Create()
	// Send the response on the callback channel to send to the server
	actionWorker.probeMsgChan <- clientMsg // This will block till the channel is ready to receive
	glog.V(3).Infof("Sent action progress for %d.", clientMsg.GetMessageID())

}

// -------------------------------- Interrupt Request Handler -----------------------------------
type InterruptMessageHandler struct {
	probes map[string]*probe.ProbeProperties
}

func (intMsgHandler *InterruptMessageHandler) HandleMessage(serverRequest proto.MediationServerMessage,
	probeMsgChan chan *proto.MediationClientMessage) {

	msgID := serverRequest.GetMessageID()
	glog.V(3).Infof("Received: Interrupt Message for message ID: %d, %s\n ", msgID, serverRequest)
}
