package mediationcontainer

import (
	"time"

	"github.com/turbonomic/turbo-go-sdk/pkg/probe"
	"github.com/turbonomic/turbo-go-sdk/pkg/proto"

	"github.com/golang/glog"
	"github.com/turbonomic/turbo-go-sdk/pkg/mediationcontainer/encoding"
	"github.com/turbonomic/turbo-go-sdk/pkg/mediationcontainer/handler"
	"github.com/turbonomic/turbo-go-sdk/pkg/mediationcontainer/transport"
)

// Abstraction to establish session using the specified protocol with the server
// and handle server messages for the different probes in the Mediation Container
type remoteMediationClient struct {
	// All the probes
	allProbes map[string]*probe.ProbeProperties
	// The container info containing the communication config for all the registered probes
	containerConfig *MediationContainerConfig
	// Associated Transport
	Transport transport.ITransport
	// Map of Message Handlers to receive server messages
	//MessageHandlers  map[handler.RequestType]handler.RequestHandler

	//
	serverRequestHandler *handler.ServerMessageHandler

	stopMsgHandlerCh chan bool
	// Channel for receiving responses from the registered probes to be sent to the server
	probeResponseChan chan *proto.MediationClientMessage
	// Channel to stop the mediation client and the underlying transport and message handling
	stopMediationClientCh chan struct{}
	//  Channel to stop the routine that monitors the underlying transport connection
	closeWatcherCh chan bool
}

func CreateRemoteMediationClient(allProbes map[string]*probe.ProbeProperties,
	containerConfig *MediationContainerConfig) *remoteMediationClient {
	remoteMediationClient := &remoteMediationClient{
		serverRequestHandler:  handler.NewServerMessageHandler(allProbes),
		allProbes:             allProbes,
		containerConfig:       containerConfig,
		probeResponseChan:     make(chan *proto.MediationClientMessage),
		stopMediationClientCh: make(chan struct{}),
	}

	glog.V(4).Infof("Created channels : probeResponseChan %s, stopMediationClientCh %s\n",
		remoteMediationClient.probeResponseChan, remoteMediationClient.stopMediationClientCh)

	glog.V(2).Infof("Created remote mediation client")

	return remoteMediationClient
}

// Establish connection with the Turbo server -  Blocks till WebSocket connection is open
// Complete the probe registration protocol with the server and then wait for server messages
func (remoteMediationClient *remoteMediationClient) Init(probeRegisteredMsgCh chan bool) {
	// TODO: Assert that the probes are registered before starting the handshake ??

	//// --------- Create WebSocket Transport
	connConfig, err := transport.CreateWebSocketConnectionConfig(remoteMediationClient.containerConfig.TurboServer,
		remoteMediationClient.containerConfig.TransportConfig)
	if err != nil {
		//
		glog.Fatalf("Initialization of remote mediation client failed, null transport : %s",  err)
		// TODO: handle error
		//remoteMediationClient.Stop()
		//probeRegisteredMsg <- false
		return
	}

	// Sdk Protocol handler
	sdkProtocolHandler := handler.CreateSdkClientProtocolHandler(remoteMediationClient.allProbes)
	// ------ WebSocket transport

	transportLayer := transport.CreateClientWebSocketTransport(connConfig) //, transportClosedNotificationCh)
	remoteMediationClient.closeWatcherCh = make(chan bool, 1)
	// Routine to monitor the WebSocket connection
	go func() {
		glog.V(3).Infof("[Reconnect] start monitoring the transport connection")
		for {
			select {
			case <-remoteMediationClient.closeWatcherCh:
				glog.V(4).Infof("[Reconnect] Exit routine *************")
				return
			case <-transportLayer.NotifyClosed():
				glog.V(2).Infof("[Reconnect] transport endpoint is closed, starting reconnect ...")

				// stop server messages listener
				remoteMediationClient.stopMessageHandler()
				// Reconnect
				err := transportLayer.Connect()
				// handle WebSocket creation errors
				if err != nil { //transport.ws == nil {
					glog.Errorf("[Reconnect] Initialization of remote mediation client failed, null transport")
					remoteMediationClient.Stop()
					break
				}
				// sdk registration protocol
				transportReady := make(chan bool, 1)
				sdkProtocolHandler.HandleClientProtocol(transportLayer, transportReady)
				endProtocol := <-transportReady
				if !endProtocol {
					glog.Errorf("[Reconnect] Registration with server failed")
					remoteMediationClient.Stop()
					break
				}
				// start listener for server messages
				remoteMediationClient.stopMsgHandlerCh = make(chan bool)
				go remoteMediationClient.RunServerMessageHandler(remoteMediationClient.Transport)
				glog.V(3).Infof("[Reconnect] transport endpoint connect complete")
			} //end select
		} // end for
	}() // end go routine
	err = transportLayer.Connect() // TODO: blocks till websocket connection is open or until transport is closed

	// handle WebSocket creation errors
	if err != nil { //transport.ws == nil {
		glog.Errorf("Initialization of remote mediation client failed, null transport")
		remoteMediationClient.Stop()
		probeRegisteredMsgCh <- false
		return
	}

	remoteMediationClient.Transport = transportLayer

	// -------- Start protocol handler separate thread
	// Initiate protocol to connect to server
	glog.V(2).Infof("Start sdk client protocol ........")
	sdkProtocolDoneCh := make(chan bool, 1) // TODO: using a channel so we can add timeout or
	// wait till message is received from the Protocol handler
	go sdkProtocolHandler.HandleClientProtocol(remoteMediationClient.Transport, sdkProtocolDoneCh)

	status := <-sdkProtocolDoneCh

	glog.V(4).Infof("Sdk client protocol complete, status = ", status)
	if !status {
		glog.Errorf("Registration with server failed")
		probeRegisteredMsgCh <- status
		remoteMediationClient.Stop()
		return
	}
	// --------- Listen for server messages
	remoteMediationClient.stopMsgHandlerCh = make(chan bool)
	go remoteMediationClient.RunServerMessageHandler(remoteMediationClient.Transport)

	// Send registration status to the upper layer
	defer close(probeRegisteredMsgCh)
	defer close(sdkProtocolDoneCh)
	probeRegisteredMsgCh <- status
	glog.V(3).Infof("Sent registration status on channel %s\n", probeRegisteredMsgCh)

	glog.V(3).Infof("Remote mediation initialization complete")
	// --------- Wait for exit notification
	select {
	case <-remoteMediationClient.stopMediationClientCh:
		glog.V(4).Infof("[Init] Exit routine *************")
		return
	}
}

// Stop the remote mediation client by closing the underlying transport and message handler routines
func (remoteMediationClient *remoteMediationClient) Stop() {
	// First stop the transport connection monitor
	close(remoteMediationClient.closeWatcherCh)
	// Stop the server message listener
	remoteMediationClient.stopMessageHandler()
	// Close the transport
	if remoteMediationClient.Transport != nil {
		remoteMediationClient.Transport.CloseTransportPoint()
	}
	// Notify the client to stop
	close(remoteMediationClient.stopMediationClientCh)
}

// ======================== Listen for server messages ===================
// Sends message to the server message listener to close the protobuf endpoint and message listener
func (remoteMediationClient *remoteMediationClient) stopMessageHandler() {
	close(remoteMediationClient.stopMsgHandlerCh)
}

// Checks for incoming server messages received by the ProtoBuf endpoint created to handle server requests
func (remoteMediationClient *remoteMediationClient) RunServerMessageHandler(transport transport.ITransport) {
	glog.V(2).Infof("[handleServerMessages] %s : ENTER  ", time.Now())

	// Create Protobuf Endpoint to handle server messages
	protoMsg := &encoding.MediationRequest{} // parser for the server requests
	endpoint := encoding.CreateClientProtoBufEndpoint("ServerRequestEndpoint", transport, protoMsg, false)
	logPrefix := "[handleServerMessages][" + endpoint.GetName() + "] : "

	// Spawn a new go routine that serves as a Callback for Probes when their response is ready
	go remoteMediationClient.runProbeCallback(endpoint) // this also exits using the stopMsgHandlerCh

	// main loop for listening to server message.
	for {
		glog.V(2).Infof(logPrefix + "waiting for parsed server message .....") // make debug
		// Wait for the server request to be received and parsed by the protobuf endpoint
		select {
		case <-remoteMediationClient.stopMsgHandlerCh:
			glog.V(4).Infof(logPrefix + "Exit routine ***************")
			endpoint.CloseEndpoint() //to stop the message listener and close the channel
			return
		case parsedMsg, ok := <-endpoint.MessageReceiver(): // block till a message appears on the endpoint's message channel
			if !ok {
				glog.Errorf(logPrefix + "endpoint message channel is closed")
				break // return or continue ?
			}
			glog.V(3).Infof(logPrefix+"received: %s\n", parsedMsg)

			// Handler response - find the handler to handle the message
			serverRequest := parsedMsg.ServerMsg
			remoteMediationClient.serverRequestHandler.HandleMessage(serverRequest,
				remoteMediationClient.probeResponseChan)
		} //end select
	} //end for
	glog.Infof(logPrefix + "DONE")
}

// Run probe callback to the probe response to the server.
// Probe responses put on the probeResponseChan by the different message handlers are sent to the server
func (remoteMediationClient *remoteMediationClient) runProbeCallback(endpoint encoding.ProtoBufEndpoint) {
	glog.V(4).Infof("[runProbeCallback] %s : ENTER  ", time.Now())
	for {
		glog.V(2).Infof("[probeCallback] waiting for probe responses")
		glog.V(4).Infof("[probeCallback] waiting for probe responses ..... on  %v\n", remoteMediationClient.probeResponseChan)
		select {
		case <-remoteMediationClient.stopMsgHandlerCh:
			glog.V(4).Infof("[probeCallback] Exit routine *************")
			return
		case msg, ok := <-remoteMediationClient.probeResponseChan:
			if !ok {
				glog.Errorf("[probeCallback] probe response channel is closed")
				break
			}
			glog.V(4).Infof("[probeCallback] received response on probe channel %v\n ", remoteMediationClient.probeResponseChan)
			endMsg := &encoding.EndpointMessage{
				ProtoBufMessage: msg,
			}
			endpoint.Send(endMsg)
		} // end select
	}
	glog.V(4).Infof("[probeCallback] DONE")
}

// ======================== Message Handlers ============================
