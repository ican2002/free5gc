package test

import (
	"errors"
	"fmt"
	"free5gc/lib/CommonConsumerTestData/UDR/TestRegistrationProcedure"
	"free5gc/lib/nas/nasConvert"
	"free5gc/lib/nas/nasMessage"
	"free5gc/lib/nas/nasType"
	"free5gc/lib/ngap/ngapSctp"
	"free5gc/lib/openapi/models"
	"git.cs.nctu.edu.tw/calee/sctp"
	"log"
	"net"
	"os"
	"sync"
)

var logger *log.Logger

func init() {
	logger = log.New(os.Stderr, "[TEST]", log.Lshortfile|log.Ltime)
	// todo
}

var currentImsiValue = int64(2089300007486)
var mx sync.Mutex

// example: imsi-2089300007484
func NewSupi() string {
	mx.Lock()
	currentImsiValue++
	v := currentImsiValue
	mx.Unlock()
	return fmt.Sprintf("imsi-%13d", v)
}

func getAccessAndMobilitySubscriptionData() (amData models.AccessAndMobilitySubscriptionData) {
	return TestRegistrationProcedure.TestAmDataTable[TestRegistrationProcedure.FREE5GC_CASE]
}

func getSmfSelectionSubscriptionData() (smfSelData models.SmfSelectionSubscriptionData) {
	return TestRegistrationProcedure.TestSmfSelDataTable[TestRegistrationProcedure.FREE5GC_CASE]
}

func getSessionManagementSubscriptionData() (smfSelData models.SessionManagementSubscriptionData) {
	return TestRegistrationProcedure.TestSmSelDataTable[TestRegistrationProcedure.FREE5GC_CASE]
}

func getAmPolicyData() (amPolicyData models.AmPolicyData) {
	return TestRegistrationProcedure.TestAmPolicyDataTable[TestRegistrationProcedure.FREE5GC_CASE]
}

func getSmPolicyData() (smPolicyData models.SmPolicyData) {
	return TestRegistrationProcedure.TestSmPolicyDataTable[TestRegistrationProcedure.FREE5GC_CASE]
}
func insertSubscriberDataToMongodb(ueId string, authSubs models.AuthenticationSubscription, servingPlmnId string) error {
	// insert mobility management data
	InsertAuthSubscriptionToMongoDB(ueId, authSubs)
	if getData := GetAuthSubscriptionFromMongoDB(ueId); getData == nil {
		return errors.New("InsertAuthSubscriptionToMongoDB error")
	}
	amData := getAccessAndMobilitySubscriptionData()
	InsertAccessAndMobilitySubscriptionDataToMongoDB(ueId, amData, servingPlmnId)
	if getData := GetAccessAndMobilitySubscriptionDataFromMongoDB(ueId, servingPlmnId); getData == nil {
		return errors.New("InsertAccessAndMobilitySubscriptionDataToMongoDB error")
	}
	amPolicyData := getAmPolicyData()
	InsertAmPolicyDataToMongoDB(ueId, amPolicyData)
	if getData := GetAmPolicyDataFromMongoDB(ueId); getData == nil {
		return errors.New("InsertAmPolicyDataToMongoDB error")
	}

	// insert session management data
	smfSelData := getSmfSelectionSubscriptionData()
	InsertSmfSelectionSubscriptionDataToMongoDB(ueId, smfSelData, servingPlmnId)
	if getData := GetSmfSelectionSubscriptionDataFromMongoDB(ueId, servingPlmnId); getData ==nil{
		return errors.New("InsertSmfSelectionSubscriptionDataToMongoDB error")
	}
	smSelData := getSessionManagementSubscriptionData()
	InsertSessionManagementSubscriptionDataToMongoDB(ueId, servingPlmnId, smSelData)
	if getData := GetSessionManagementDataFromMongoDB(ueId, servingPlmnId); getData == nil{
		return errors.New("InsertSessionManagementSubscriptionDataToMongoDB error")
	}
	smPolicyData := getSmPolicyData()
	InsertSmPolicyDataToMongoDB(ueId, smPolicyData)
	if getData := GetSmPolicyDataFromMongoDB(ueId); getData == nil{
		return errors.New("InsertSmPolicyDataToMongoDB error")
	}

	return nil
}

func deleteNgRanSubscriberFromMongodb(supi string, servingPlmnId string) error {
	// delete test data
	DelAuthSubscriptionToMongoDB(supi)
	DelAccessAndMobilitySubscriptionDataFromMongoDB(supi, servingPlmnId)
	DelSmfSelectionSubscriptionDataFromMongoDB(supi, servingPlmnId)
	// todo
	return nil
}

// supi example: imsi-2089300007487
// 5g guti example: plmn+gumai+tmsi
// idType: suci, 5g-guti
func ueIdToMobileIdentity5GS(ueId string, typeOfId uint8) nasType.MobileIdentity5GS {
	switch typeOfId {
	case nasMessage.MobileIdentity5GSTypeSuci:
		mobileIdentity5GS := nasType.MobileIdentity5GS{
			Len:    12, // supi
			Buffer: []uint8{0x01, 0x02, 0xf8, 0x39, 0xf0, 0xff, 0x00, 0x00, 0x00, 0x00, 0x47, 0x78},	// 0x01 means imsi supi
		}
		// BCD code of ueId, plmnId must be 20893
		j := 0
		for i := len(ueId) - 1; i > len(ueId)-8; i = i - 2 {
			mobileIdentity5GS.Buffer[11-j] = (ueId[i]-'0')<<4|(ueId[i-1]-'0')
			j++
		}
		return mobileIdentity5GS
	case nasMessage.MobileIdentity5GSType5gGuti:
		mobileIdentity5GS := nasType.MobileIdentity5GS{
			Len:    11, // 5g-guti
			//Buffer: []uint8{0x02, 0x02, 0xf8, 0x39, 0xca, 0xfe, 0x00, 0x00, 0x00, 0x00, 0x01},
			Buffer: nasConvert.Guti5GToNas(ueId).Octet[:],
		}
		return mobileIdentity5GS
	default:
		logger.Fatalf("not supported mobileIdentity5GS type[%d]\n", typeOfId)
	}

	return nasType.MobileIdentity5GS{}
}

func connectToAmf(ranIP string, ranPort int, amfIP string, amfPort int) (*sctp.SCTPConn, error) {
	amfAddr, ranAddr, err := getNgapIp(amfIP, ranIP, amfPort, ranPort)
	if err != nil {
		return nil, err
	}
	conn, err := sctp.DialSCTP("sctp", ranAddr, amfAddr)
	if err != nil {
		return nil, err
	}
	info, _ := conn.GetDefaultSentParam()
	info.PPID = ngapSctp.NGAP_PPID
	err = conn.SetDefaultSentParam(info)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func getNgapIp(amfIP, ranIP string, amfPort, ranPort int) (amfAddr, ranAddr *sctp.SCTPAddr, err error) {
	ips := []net.IPAddr{}
	if ip, err1 := net.ResolveIPAddr("ip", amfIP); err1 != nil {
		err = fmt.Errorf("error resolving address '%s': %v", amfIP, err1)
		return
	} else {
		ips = append(ips, *ip)
	}
	amfAddr = &sctp.SCTPAddr{
		IPAddrs: ips,
		Port:    amfPort,
	}
	ips = []net.IPAddr{}
	if ip, err1 := net.ResolveIPAddr("ip", ranIP); err1 != nil {
		err = fmt.Errorf("error resolving address '%s': %v", ranIP, err1)
		return
	} else {
		ips = append(ips, *ip)
	}
	ranAddr = &sctp.SCTPAddr{
		IPAddrs: ips,
		Port:    ranPort,
	}
	return
}
