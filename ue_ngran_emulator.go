package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"free5gc/lib/nas"
	"free5gc/lib/nas/nasConvert"
	"free5gc/lib/nas/nasMessage"
	"free5gc/lib/nas/nasTestpacket"
	"free5gc/lib/nas/security"
	"free5gc/lib/ngap"
	"free5gc/lib/ngap/ngapType"
	"free5gc/lib/openapi/models"
	"git.cs.nctu.edu.tw/calee/sctp"
	"log"
	"net"
	"strconv"
	"time"
)

const (
	NgRanStatusOffline = iota
	NgRanStatusOnline
)

var (
	currentNgRanPort = 9487
	currentNgRanId   = 0x40000
)

type UeContext struct {
	RanUeCtx      *RanUeContext         `json:"ranUeContext"`
	NasUeCtx      *NasUeContext         `json:"nasContext"`
	UePduSessions [2]*PduSessionContext `json:"nasUePduSessionContexts"` // save pduSession Info in NAS and AS
 }

func NewUeContext(supi string, ranUeNgapId int64, snssai *models.Snssai) *UeContext {
	return &UeContext{
		RanUeCtx: NewRanUeContext(supi, ranUeNgapId, security.AlgCiphering128NEA0, security.AlgIntegrity128NIA2),
		NasUeCtx: NewNasUeContext(snssai),
	}
}

func (ueCtx *UeContext)addPduSession (pduSession *PduSessionContext) error{
	if ueCtx.UePduSessions[0] == nil{
		ueCtx.UePduSessions[0] = pduSession
		return nil
	}else if ueCtx.UePduSessions[1] == nil{
		ueCtx.UePduSessions[1] = pduSession
		return nil
	}
	return errors.New("only support two pdu session")
}

type NasUeContext struct {
	// static data
	// todo
	// dynamic data
	Guti5G string         `json:"guti5G"`
	Status int            `json:"status"`
	Snssai *models.Snssai `json:"snssai"`
}

const (
	NasUeStatusDeRegistration int = iota
	NasUeStatusRegistrationActivated
	NasUeStatusRegistrationIdle
)

func NewNasUeContext (snssai *models.Snssai) *NasUeContext{
	return &NasUeContext{
		Status: NasUeStatusDeRegistration,
		Snssai: snssai,
	}
}

type PduSessionContext struct {
	PduSessionId uint8          `json:"pduSessionId"`
	Dnn          string         `json:"dnn"`
	Snssai       *models.Snssai `json:"snssai"`
	// dynamic data
	Status int          `json:"status"`
	NasMsg *nas.Message `json:"nasMsg"`
	PDUSessionResourceSetupRequestTransfer []byte	`json:"pduSessionResourceSetupRequestTransfer"`
	PduAddr []byte	`json:"pduAddr"`
	// TODO
}

const (
	PduSessionStatusInNoActivate int = iota
	PduSessionStatusInActivating
	PduSessionStatusInActivated
	PduSessionStatusInIdle
)

const (
	DataPduSessionId uint8 = 10
	VoicePduSessionId uint8 = 11
)

func NewPduSessionContext (id uint8, dnn string, snssai *models.Snssai)*PduSessionContext{
	return &PduSessionContext{
		PduSessionId: id,
		Dnn:          dnn,
		Snssai:       snssai,
		Status:       PduSessionStatusInActivating,
	}
}

type NgRan struct {
	// global configure data
	ServingPlmn        string		`json:"servingPlmn"`
	CurrentRanUeNgapId int64		`json:"currentRanUeNgapId"`
	InitAmfUeNgapId    int64		`json:"initAmfUeNgapId"`
	ServingSnssai	*models.Snssai	`json:"servingSnssai"`
	// initAmfNgapId int64
	// supportedTai
	// supportedCells
	// static data
	N2Cfg struct{
		AmfIp     string			`json:"amfIp"`
		AmfPort   int				`json:"amfPort"`
		NgRanIp   string			`json:"ngRanIp"`
		NgRanPort int				`json:"ngRanPort"`
		GNBId     int				`json:"gnbId"`
		GNBName   string			`json:"gnbName"`
	}
	N3Cfg struct{
		UpfIP       string			`json:"upfIp"`
		UpfPort     int				`json:"upfPort"`
		NgRanUpIp   string			`json:"ngRanUpIp"`
		NgRanUpPort int				`json:"ngRanUpPort"`
	}
	// dynamic data
	Status     int					`json:"status"`
	SctpConn   *sctp.SCTPConn		`json:"sctpConn"`
	UpConn     *net.UDPConn			`json:"upConn"`
	UeContexts map[string]*UeContext `json:"ueContexts"`// key is supi
}

func NewNgRan() *NgRan {
	newNgRan := &NgRan{}
	newNgRan.ServingPlmn = "20893"
	newNgRan.CurrentRanUeNgapId = 0
	newNgRan.InitAmfUeNgapId = 1
	newNgRan.ServingSnssai = &models.Snssai{Sst: 1,	Sd:  "010203"}
	newNgRan.N2Cfg.AmfIp = "127.0.0.1"
	newNgRan.N2Cfg.AmfPort = 38412
	newNgRan.N2Cfg.NgRanIp = "127.0.0.1"
	newNgRan.N2Cfg.NgRanPort = currentNgRanPort
	currentNgRanPort++
	newNgRan.N2Cfg.GNBId = currentNgRanId
	currentNgRanId++
	newNgRan.N2Cfg.GNBName = fmt.Sprintf("ngRan-%d",newNgRan.N2Cfg.GNBId)
	newNgRan.N3Cfg.UpfIP = "10.200.200.102"
	newNgRan.N3Cfg.UpfPort = 2152
	newNgRan.N3Cfg.NgRanUpIp = "10.200.200.1"
	newNgRan.N3Cfg.NgRanUpPort = 2152
	newNgRan.Status = NgRanStatusOffline
	newNgRan.UeContexts = make(map[string]*UeContext)
	return newNgRan
}

// Connect to AMF, and setup NgAp connection and UPF tunnel
func (ngRan *NgRan) NGSetup() (err error){
	var n int
	var sendMsg []byte
	var recvMsg = make([]byte, 2048)

	// RAN connect to AMF
	ngRan.SctpConn, err = connectToAmf(ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort, ngRan.N2Cfg.AmfIp, ngRan.N2Cfg.AmfPort)
	if err != nil{
		return err
	}
	// send NGSetupRequest Msg
	sendMsg, err = GetNGSetupRequest(ngRan.gnbIdToBytes(), 24, ngRan.N2Cfg.GNBName)
	if err != nil{
		return err
	}
	if _, err = ngRan.SctpConn.Write(sendMsg); err != nil{
		return err
	}
	// receive NGSetupResponse Msg
	if n, err = ngRan.SctpConn.Read(recvMsg); err != nil{
		return err
	}
	if _, err = ngap.Decoder(recvMsg[:n]); err != nil{
		return err
	}

	//// RAN connect to UPF
	//ngRan.UpConn, err = ConnectToUpf(ngRan.N3Cfg.NgRanUpIp, ngRan.N3Cfg.UpfIP, ngRan.N3Cfg.NgRanUpPort, ngRan.N3Cfg.UpfPort)
	//if err != nil{
	//	logger.Printf("failed to establish tunnel between ngRan and UPF, because of %v\n", err)
	//	ngRan.SctpConn.Close()
	//	return err
	//}

	// set Status to online
	ngRan.Status = NgRanStatusOnline
	return nil
}

func (ngRan *NgRan) NGReset() (err error) {
	if ngRan.Status == NgRanStatusOffline {
		return nil
	}
	var err1, err2 error
	if ngRan.SctpConn != nil{
		err1 = ngRan.SctpConn.Close()
	}
	if ngRan.UpConn != nil{
		err2 = ngRan.UpConn.Close()
	}
	ngRan.Status = NgRanStatusOffline
	if err1 != nil || err2 != nil{
		return errors.New(fmt.Sprintf("close sctp conn err:%v; close upp conn err:%v", err1, err2))
	}
	return nil
}

// supi example: "imsi-2089300007487"
func (ngRan *NgRan) Registration (supi string) (err error){
	if ngRan.Status == NgRanStatusOffline {
		return errors.New("ngRAN is offline")
	}

	var n int
	var sendMsg []byte
	var recvMsg = make([]byte, 2048)

	// New UE Context
	// RanUeContext := NewRanUeContext("imsi-2089300007487", 1, security.AlgCiphering128NEA2 NEA2 or NEA0?, security.AlgIntegrity128NIA2)
	ueCtx := NewUeContext(supi, ngRan.newRanUeNgapId(),ngRan.ServingSnssai)
	ueCtx.RanUeCtx.AmfUeNgapId = ngRan.InitAmfUeNgapId
	ueCtx.RanUeCtx.AuthenticationSubs = getAuthSubscription()
	// insert RanUeContext to MongoDB
	if err = insertSubscriberDataToMongodb(supi, ueCtx.RanUeCtx.AuthenticationSubs, ngRan.ServingPlmn); err != nil{
		return err
	}
	// send InitialUeMessage(Registration Request)(imsi-2089300007487)
	ueSecurityCapability := setUESecurityCapability(ueCtx.RanUeCtx)
	registrationRequest := nasTestpacket.GetRegistrationRequestWith5GMM(nasMessage.RegistrationType5GSInitialRegistration, ueIdToMobileIdentity5GS(supi, nasMessage.MobileIdentity5GSTypeSuci), nil, nil, ueSecurityCapability)
	if sendMsg, err = GetInitialUEMessage(ueCtx.RanUeCtx.RanUeNgapId, registrationRequest, ""); err ==nil{
		if _, err = ngRan.SctpConn.Write(sendMsg); err != nil{
			return err
		}
	}else {
		return err
	}
	// receive NAS Authentication Request Msg
	n, err = ngRan.SctpConn.Read(recvMsg)
	if err != nil{
		return err
	}
	ngapMsg, err := ngap.Decoder(recvMsg[:n])
	if err != nil{
		return err
	}
	// Calculate for RES*
	nasPdu := GetNasPdu(ngapMsg.InitiatingMessage.Value.DownlinkNASTransport)
	if nasPdu == nil{
		return errors.New("GetNasPdu (NAS Authentication Request Msg) error")
	}
	rand := nasPdu.AuthenticationRequest.GetRANDValue()
	resStat := ueCtx.RanUeCtx.DeriveRESstarAndSetKey(ueCtx.RanUeCtx.AuthenticationSubs, rand[:], "5G:mnc093.mcc208.3gppnetwork.org")
	// send NAS Authentication Response
	pdu := nasTestpacket.GetAuthenticationResponse(resStat, "")
	sendMsg, err = GetUplinkNASTransport(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, pdu)
	if err != nil{
		return err
	}
	if _, err = ngRan.SctpConn.Write(sendMsg); err != nil{
		return err
	}
	// receive NAS Security Mode Command Msg
	n, err = ngRan.SctpConn.Read(recvMsg)
	if err != nil{
		return err
	}
	if _, err = ngap.Decoder(recvMsg[:n]); err != nil{
		return err
	}
	// send NAS Security Mode Complete Msg
	pdu = nasTestpacket.GetSecurityModeComplete(registrationRequest)
	pdu, err = EncodeNasPduWithSecurity(ueCtx.RanUeCtx, pdu, nas.SecurityHeaderTypeIntegrityProtectedAndCipheredWithNew5gNasSecurityContext, true, true)
	if err != nil{
		return err
	}
	sendMsg, err = GetUplinkNASTransport(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, pdu)
	if err != nil{
		return err
	}
	if _, err = ngRan.SctpConn.Write(sendMsg); err != nil{
		return err
	}
	// receive ngap Initial Context Setup Request Msg
	n, err = ngRan.SctpConn.Read(recvMsg)
	if err != nil{
		return err
	}
	ngapMsg, err = ngap.Decoder(recvMsg[:n])
	if err != nil{
		return err
	}
	// get AmfUeNgapId
	if err = ngRan.handleInitialContextSetupRequest(ueCtx, ngapMsg); err != nil{
		return err
	}
	// send ngap Initial Context Setup Response Msg
	sendMsg, err = GetInitialContextSetupResponse(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId)
	if err != nil{
		return err
	}
	if _, err = ngRan.SctpConn.Write(sendMsg); err != nil{
		return err
	}
	// send NAS Registration Complete Msg
	pdu = nasTestpacket.GetRegistrationComplete(nil)
	pdu, err = EncodeNasPduWithSecurity(ueCtx.RanUeCtx, pdu, nas.SecurityHeaderTypeIntegrityProtectedAndCiphered, true, false)
	if err != nil{
		return err
	}
	sendMsg, err = GetUplinkNASTransport(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, pdu)
	if err != nil{
		return err
	}
	if _, err = ngRan.SctpConn.Write(sendMsg); err != nil{
		return err
	}

	ngRan.addUeContext(supi, ueCtx)
	return
}

func (ngRan *NgRan) DeRegistration (supi string) (err error){
	var n int
	var sendMsg []byte
	var recvMsg = make([]byte, 2048)

	ueCtx, ok := ngRan.UeContexts[supi]
	if !ok{
		return errors.New(fmt.Sprintf("ue [%s] does not exist\n", supi))
	}

	// send NAS de-registration Request (UE Originating)
	//mobileIdentity5GS := nasType.MobileIdentity5GS{
	//	Len:    11, // 5g-guti
	//	Buffer: nasConvert.Guti5GToNas(ueCtx.NasUeCtx.Guti5G).Octet[:],
	//}
	pdu := nasTestpacket.GetDeregistrationRequest(nasMessage.AccessType3GPP, 0, 0x04, ueIdToMobileIdentity5GS(ueCtx.NasUeCtx.Guti5G,nasMessage.MobileIdentity5GSType5gGuti))
	pdu, err = EncodeNasPduWithSecurity(ueCtx.RanUeCtx, pdu, nas.SecurityHeaderTypeIntegrityProtectedAndCiphered, true, false)
	if err != nil{
		return err
	}
	sendMsg, err = GetUplinkNASTransport(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, pdu)
	if err != nil{
		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		return err
	}
	// receive DeRegistration Accept
	n, err = ngRan.SctpConn.Read(recvMsg)
	if err != nil{
		return err
	}
	// process DeRegistrationAccept message
	// todo
	// receive ngap UE Context Release Command
	n, err = ngRan.SctpConn.Read(recvMsg)
	if err != nil{
		return err
	}
	ngapPdu, err := ngap.Decoder(recvMsg[:n])
	if err != nil{
		return err
	}
	if err = ngRan.handleUeContextReleaseCommand(ueCtx, ngapPdu); err != nil{
		return err
	}
	// send ngap UE Context Release Complete
	sendMsg, err = GetUEContextReleaseComplete(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, nil)
	_, err = ngRan.SctpConn.Write(sendMsg)

	// delete associated date
	ngRan.deleteSubscriberTestDataFromMongodb(supi)
	ngRan.deleteUeContext(supi)

	return nil
}

func (ngRan *NgRan) EstablishPduSession (supi string, pduSessionId uint8, dnn string) (err error){
	var n int
	var sendMsg []byte
	var recvMsg = make([]byte, 2048)

	ueCtx, ok := ngRan.UeContexts[supi]
	if !ok{
		return errors.New(fmt.Sprintf("ue [%s] does not exist\n", supi))
	}

	if err = ueCtx.addPduSession(NewPduSessionContext(pduSessionId,dnn, ngRan.ServingSnssai)); err != nil{
		return err
	}
	// send GetPduSessionEstablishmentRequest Msg
	nasPdu := nasTestpacket.GetUlNasTransport_PduSessionEstablishmentRequest(pduSessionId, nasMessage.ULNASTransportRequestTypeInitialRequest, dnn, ngRan.ServingSnssai)
	nasPdu, err = EncodeNasPduWithSecurity(ueCtx.RanUeCtx, nasPdu, nas.SecurityHeaderTypeIntegrityProtectedAndCiphered, true, false)
	if err != nil{
		return err
	}
	sendMsg, err = GetUplinkNASTransport(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, nasPdu)
	if err != nil{
		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		return err
	}
	// receive 12. NGAP-PDU Session Resource Setup Request(DL nas transport((NAS msg-PDU session setup Accept)))
	n, err = ngRan.SctpConn.Read(recvMsg)
	if err != nil{
		return err
	}
	ngapPdu, err := ngap.Decoder(recvMsg[:n])
	if err != nil{
		return err
	}
	if err = ngRan.handlePduSessionResourceSetupRequest(ueCtx, ngapPdu); err != nil{
		return err
	}
	// send 14. NGAP-PDU Session Resource Setup Response
	sendMsg, err = GetPDUSessionResourceSetupResponse(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, ngRan.N2Cfg.NgRanIp)
	if err != nil{
		logger.Printf("failed to GetPDUSessionResourceSetupResponse, because of %v\n", err)
		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		logger.Printf("failed to send 14. NGAP-PDU Session Resource Setup Response, because of %v\n", err)
		return err
	}

	return nil
}

func (ngRan *NgRan) ModifyPduSession (supi string, pduSessionId uint8) error{
	// todo
	return nil
}

func (ngRan *NgRan) ReleasePduSession (supi string, pduSessionId uint8) (err error){
	//var n int
	var sendMsg []byte
	//var recvMsg = make([]byte, 2048)

	ueCtx, ok := ngRan.UeContexts[supi]
	if !ok{
		return errors.New(fmt.Sprintf("ue [%s] does not exist\n", supi))
	}
	found := false
	var pduSessionCtx *PduSessionContext
	for _, pduSessionCtx = range ueCtx.UePduSessions {
		if pduSessionCtx.PduSessionId == pduSessionId{
			found = true
			break
		}
	}
	if !found{
		logger.Printf("pdu session [id = %d] of ue [%s] is not exist\n",pduSessionId, supi)
		return errors.New(fmt.Sprintf("pdu session[%d] not exist",pduSessionId))
	}

	// Send Pdu Session Release Request
	nasPdu := nasTestpacket.GetUlNasTransport_PduSessionReleaseRequest(pduSessionId)
	nasPdu, err = EncodeNasPduWithSecurity(ueCtx.RanUeCtx, nasPdu, nas.SecurityHeaderTypeIntegrityProtectedAndCiphered, true, false)
	if err != nil{
		logger.Printf("failed to EncodeNasPduWithSecurity, because of %v\n", err)
		return err
	}
	sendMsg, err = GetUplinkNASTransport(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, nasPdu)
	if err != nil{
		logger.Printf("failed to GetUplinkNASTransport, because of %v\n", err)
		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		logger.Printf("failed to Send Pdu Session Release Request, because of %v\n", err)
		return err
	}

	time.Sleep(time.Millisecond*10)

	// send N2 Resource Release Ack(PDUSession Resource Release Response)
	sendMsg, err = GetPDUSessionResourceReleaseResponse(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId)
	if err != nil{
		logger.Printf("failed to GetPDUSessionResourceReleaseResponse, because of %v\n", err)
		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		logger.Printf("failed to send N2 Resource Release Ack(PDUSession Resource Release Response), because of %v\n", err)
		return err
	}

	// wait 10 ms
	time.Sleep(10 * time.Millisecond)

	//send N1 PDU Session Release Ack PDU session release complete
	nasPdu = nasTestpacket.GetUlNasTransport_PduSessionReleaseComplete(10, nasMessage.ULNASTransportRequestTypeInitialRequest, pduSessionCtx.Dnn, pduSessionCtx.Snssai)
	nasPdu, err = EncodeNasPduWithSecurity(ueCtx.RanUeCtx, nasPdu, nas.SecurityHeaderTypeIntegrityProtectedAndCiphered, true, false)
	if err != nil{
		logger.Printf("failed to EncodeNasPduWithSecurity, because of %v\n", err)
		return err
	}
	sendMsg, err = GetUplinkNASTransport(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, nasPdu)
	if err != nil{
		logger.Printf("failed to GetUplinkNASTransport, because of %v\n", err)
		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		logger.Printf("failed to send N1 PDU Session Release Ack PDU session release complete, because of %v\n", err)
		return err
	}

	return ngRan.deletePduSession(supi, pduSessionId)
}

func (ngRan *NgRan) N2Release (supi string) (err error){
	var n int
	var sendMsg []byte
	var recvMsg = make([]byte, 2048)

	ueCtx, ok := ngRan.UeContexts[supi]
	if !ok{
		return errors.New(fmt.Sprintf("ue [%s] does not exist\n", supi))
	}

	// send ngap UE Context Release Request
	pduSessionIDList := make([]int64,0,2)
	for _, pduSessionContext := range ueCtx.UePduSessions{
		if pduSessionContext != nil{
			pduSessionIDList = append(pduSessionIDList, int64(pduSessionContext.PduSessionId))
		}
	}
	sendMsg, err = GetUEContextReleaseRequest(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, pduSessionIDList)
	if err != nil{
		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		return err
	}
	// receive UE Context Release Command
	n, err = ngRan.SctpConn.Read(recvMsg)
	if err != nil{
		return err
	}
	ngapPdu, err := ngap.Decoder(recvMsg[:n])
	if err != nil{
		return err
	}
	err = ngRan.handleUeContextReleaseCommand(ueCtx,ngapPdu)
	if err != nil{
		return err
	}
	// send ngap UE Context Release Complete
	sendMsg, err = GetUEContextReleaseComplete(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, nil)
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		return err
	}

	return nil
}

func (ngRan *NgRan) ServiceRequest (supi string, pduSessionId uint8) (err error){
	//var n int
	var sendMsg []byte
	var recvMsg = make([]byte, 2048)

	ueCtx, ok := ngRan.UeContexts[supi]
	if !ok{
		return errors.New(fmt.Sprintf("ue [%s] does not exist\n", supi))
	}
	if (ueCtx.UePduSessions[0] != nil && ueCtx.UePduSessions[0].PduSessionId != pduSessionId) || (ueCtx.UePduSessions[1] != nil && ueCtx.UePduSessions[1].PduSessionId != pduSessionId){
		return errors.New(fmt.Sprintf("pduSession[%d] not exist",pduSessionId))
	}

	// re-allocate a RanUeNgapId when initiating a service request in idle
	if ueCtx.NasUeCtx.Status == NasUeStatusRegistrationIdle{
		ueCtx.RanUeCtx.RanUeNgapId = ngRan.newRanUeNgapId()
	}
	// send NAS Service Request
	pdu := nasTestpacket.GetServiceRequest(nasMessage.ServiceTypeData)
	pdu, err = EncodeNasPduWithSecurity(ueCtx.RanUeCtx, pdu, nas.SecurityHeaderTypeIntegrityProtectedAndCiphered, true, false)
	if err != nil{
		log.Println(err)
		return err
	}
	sendMsg, err = GetInitialUEMessage(ueCtx.RanUeCtx.RanUeNgapId, pdu, nasConvert.Guti5GDeriveTmsi5G(ueCtx.NasUeCtx.Guti5G)) //"fe0000000001",
	if err != nil{
		logger.Println(err)

		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		logger.Println(err)

		return err
	}
	// receive Initial Context Setup Request
	_, err = ngRan.SctpConn.Read(recvMsg)
	if err != nil{
		logger.Println(err)

		return err
	}
	//ngapPdu, err := ngap.Decoder(recvMsg[:n])
	//if err != nil{
	//	return err
	//}
	//if err = ngRan.handleInitialContextSetupRequest(ueCtx, ngapPdu); err !=nil{
	//	logger.Println(err)
	//
	//	return err
	//}
	//time.Sleep(time.Second)
	// Send Initial Context Setup Response
	sendMsg, err = GetInitialContextSetupResponseForServiceRequest(ueCtx.RanUeCtx.AmfUeNgapId, ueCtx.RanUeCtx.RanUeNgapId, ngRan.N2Cfg.NgRanIp)
	if err != nil{
		return err
	}
	_, err = ngRan.SctpConn.Write(sendMsg)
	if err != nil{
		return err
	}

	return nil
}

func (ngRan *NgRan) handleInitialContextSetupRequest (ueContext *UeContext, ngapPdu *ngapType.NGAPPDU) error{
	if ngapPdu.Present != ngapType.NGAPPDUPresentInitiatingMessage || ngapPdu.InitiatingMessage.Value.Present != ngapType.InitiatingMessagePresentInitialContextSetupRequest{
		return errors.New(fmt.Sprintf("InitialContextSetupRequest message format error[%v]",ngapPdu))
	}
	for _, ie := range ngapPdu.InitiatingMessage.Value.InitialContextSetupRequest.ProtocolIEs.List{
		switch ie.Id.Value{
		case ngapType.ProtocolIEIDAMFUENGAPID:
			ueContext.RanUeCtx.AmfUeNgapId = ie.Value.AMFUENGAPID.Value
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID.Value != ueContext.RanUeCtx.RanUeNgapId{
				logger.Printf("RanUeNgapId is change to %d from %d\n",ie.Value.RANUENGAPID.Value, ueContext.RanUeCtx.RanUeNgapId)
				ueContext.RanUeCtx.RanUeNgapId = ie.Value.RANUENGAPID.Value
			}
		case ngapType.ProtocolIEIDPDUSessionResourceSetupListCxtReq:
			// TODO
		case ngapType.ProtocolIEIDPDUSessionResourceSetupListSUReq:
			// TODO
		// handle NAS PDU
		case ngapType.ProtocolIEIDNASPDU:
			pkg := []byte(ie.Value.NASPDU.Value)
			m, err := NASDecode(ueContext.RanUeCtx, nas.GetSecurityHeaderType(pkg), pkg)
			if err != nil{
				return errors.New(fmt.Sprintf("nas message included initialContextSetupRequest format error [%v]",pkg))
			}
			if m.GmmMessage != nil{			// GMM Message
				switch m.GmmMessage.GetMessageType() {
				case nas.MsgTypeRegistrationAccept:
					if m.GmmMessage.RegistrationAccept.RegistrationResult5GS.Octet != 0x001{		// 3gpp access
						// todo
					}
					_, ueContext.NasUeCtx.Guti5G = nasConvert.GutiToString(m.GmmMessage.RegistrationAccept.GUTI5G.Octet[:])
					ueContext.NasUeCtx.Status = NasUeStatusRegistrationActivated
				default:
					return errors.New(fmt.Sprintf("not supported gmm message type[%d]",m.GmmMessage.GetMessageType()))
				}
			}else if m.GsmMessage != nil{
				switch m.GsmMessage.GetMessageType() {
				case 3:
				default:
					return errors.New(fmt.Sprintf("not supported gsm message type[%d]",m.GsmMessage.GetMessageType()))
				}
			}
		default:
			if ie.Id.Value > ngapType.ProtocolIEIDWarningAreaCoordinates{
				return errors.New(fmt.Sprintf("not supported ngap IEI[%d]\n",ie.Id.Value))
			}
		}
	}
	return nil
}

func (ngRan *NgRan) handlePduSessionResourceSetupRequest (ueContext *UeContext, ngapPdu *ngapType.NGAPPDU) error{
	if ngapPdu.Present != ngapType.NGAPPDUPresentInitiatingMessage || ngapPdu.InitiatingMessage.Value.Present != ngapType.InitiatingMessagePresentPDUSessionResourceSetupRequest{
		return errors.New(fmt.Sprintf("PduSessionResourceSetupRequest message format error[%v]",ngapPdu))
	}

	for _, ie := range ngapPdu.InitiatingMessage.Value.PDUSessionResourceSetupRequest.ProtocolIEs.List{
		switch ie.Id.Value{
		case ngapType.ProtocolIEIDAMFUENGAPID:
			ueContext.RanUeCtx.AmfUeNgapId = ie.Value.AMFUENGAPID.Value
		case ngapType.ProtocolIEIDRANUENGAPID:
			if ie.Value.RANUENGAPID.Value != ueContext.RanUeCtx.RanUeNgapId{
				return errors.New(fmt.Sprintf("requested RanUeNgapId = %d, it should be %d",ie.Value.RANUENGAPID.Value, ueContext.RanUeCtx.RanUeNgapId))
			}
		case ngapType.ProtocolIEIDPDUSessionResourceSetupListCxtReq:
			for _, pduSessionResourceSetupRequest := range ie.Value.PDUSessionResourceSetupListSUReq.List{
				var pduSessionContext *PduSessionContext
				switch uint8(pduSessionResourceSetupRequest.PDUSessionID.Value) {
				case ueContext.UePduSessions[0].PduSessionId:
					pduSessionContext = ueContext.UePduSessions[0]
				case ueContext.UePduSessions[1].PduSessionId:
					pduSessionContext = ueContext.UePduSessions[1]
				default:
					return errors.New(fmt.Sprintf("pdu session id[%d] not exist",pduSessionResourceSetupRequest.PDUSessionID.Value))
				}
				pduSessionContext.PDUSessionResourceSetupRequestTransfer = pduSessionResourceSetupRequest.PDUSessionResourceSetupRequestTransfer[:]
				if pduSessionResourceSetupRequest.PDUSessionNASPDU != nil{
					pkg := pduSessionResourceSetupRequest.PDUSessionNASPDU.Value
					m, err := NASDecode(ueContext.RanUeCtx, nas.GetSecurityHeaderType(pkg), pkg)
					if err != nil{
						return errors.New(fmt.Sprintf("nas message included initialContextSetupRequest format error [%v]",pkg))
					}
					pduSessionContext.NasMsg = m
					if m.GsmMessage != nil{
						switch m.GsmMessage.GetMessageType() {
						case nas.MsgTypePDUSessionEstablishmentAccept:
							pduSessionContext.PduAddr = m.GsmMessage.PDUSessionEstablishmentAccept.PDUAddress.Octet[:]
							pduSessionContext.Status = PduSessionStatusInActivated
						default:
							return errors.New(fmt.Sprintf("not supported gsm message type[%d]",m.GsmMessage.GetMessageType()))
						}
					}
				}
			}
		case ngapType.ProtocolIEIDPDUSessionResourceModifyListModReq:
			// TODO
		default:
			if ie.Id.Value > ngapType.ProtocolIEIDWarningAreaCoordinates{
				return errors.New(fmt.Sprintf("not supported ngap IEI[%d]\n",ie.Id.Value))
			}
		}
	}
	return nil
}

func (ngRan *NgRan) handleUeContextReleaseCommand (ueContext *UeContext, ngapPdu *ngapType.NGAPPDU) error{
	if ngapPdu.Present != ngapType.NGAPPDUPresentInitiatingMessage || ngapPdu.InitiatingMessage.Value.Present != ngapType.InitiatingMessagePresentUEContextReleaseCommand{
		return errors.New(fmt.Sprintf("UeContextRelease message format error[%v]",ngapPdu.InitiatingMessage))
	}
	// simple process the message, release all pduSessions
	if ueContext.UePduSessions[0] != nil {
		ueContext.UePduSessions[0].Status = PduSessionStatusInIdle
	}
	if ueContext.UePduSessions[1] != nil{
		ueContext.UePduSessions[1].Status = PduSessionStatusInIdle
	}
	// if all pdusession is idle, status of ue is set to idle
	ueContext.NasUeCtx.Status = NasUeStatusRegistrationIdle

	return nil
}

func (ngRan *NgRan) newRanUeNgapId() int64{
	ngRan.CurrentRanUeNgapId++
	return ngRan.CurrentRanUeNgapId
}

func (ngRan *NgRan) addUeContext (supi string, ueCtx *UeContext){
	ngRan.UeContexts[supi] = ueCtx
}

func (ngRan *NgRan) deletePduSession (supi string, id uint8) error{
	if ueCtx, ok := ngRan.UeContexts[supi]; ok{
		ueCtx.UePduSessions[0] = nil
		return nil
	}
	return errors.New(fmt.Sprintf("ue [%s] not exist",supi))
}

func (ngRan *NgRan) deleteUeContext (supi string){
	delete(ngRan.UeContexts, supi)
}

func (ngRan *NgRan) deleteSubscriberTestDataFromMongodb(supi string){
	// delete test data
	DelAuthSubscriptionToMongoDB(supi)
	DelAccessAndMobilitySubscriptionDataFromMongoDB(supi, ngRan.ServingPlmn)
	DelSmfSelectionSubscriptionDataFromMongoDB(supi, ngRan.ServingPlmn)
}

func (ngRan *NgRan) gnbIdToBytes () []byte{
	return []byte(strconv.Itoa(ngRan.N2Cfg.GNBId))
}
//
func (ngRan *NgRan) ShowUeContext(supi string) string{
	if ueCtx, ok := ngRan.UeContexts[supi]; ok{
		data, _ := json.MarshalIndent(ueCtx, "", "  ")
		return string(data)
	}
	return ""
}

func (ngRan *NgRan) String () string{
	data, err := json.MarshalIndent(ngRan, "", "  ")
	if err != nil{
		logger.Println(err)
	}
	return string(data)
}