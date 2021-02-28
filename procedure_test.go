package test_test

import (
	"free5gc/src/test"
	"testing"
	"time"
)

const (
	NumberOfNgRan    int = 1
	UeNumberPerNgRan int = 100
)

func TestNGSetupAndNGReset(t *testing.T) {
	ngRans := make([]*test.NgRan, 0, NumberOfNgRan)
	startTime := time.Now()
	t.Log("Starting connect to AMF ...")
	for i := 0; i < NumberOfNgRan; i++ {
		ngRan := test.NewNgRan()
		err := ngRan.NGSetup()
		if err != nil {
			t.Fatalf("failed to connect AMF, because of [%v]\n", err)
		}
		t.Logf("ngRan[%s:%d] connected to AMF [%s:%d]\n", ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort, ngRan.N2Cfg.AmfIp, ngRan.N2Cfg.AmfPort)
		ngRans = append(ngRans, ngRan)
	}

	time.Sleep(time.Second)

	t.Log("Starting disconnect to AMF ...")
	for _, ngRan := range ngRans {
		if err := ngRan.NGReset(); err != nil {
			t.Fatalf("ngRan[%s:%d] failed to disconnect to AMF\n", ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort)
		}
	}
	t.Logf("escape time:%dms for %d times NGSetup and NGReset\n", time.Since(startTime).Milliseconds(), NumberOfNgRan)
}

func TestRegistrationAndDeRegistration(t *testing.T) {
	ngRan := test.NewNgRan()
	err := ngRan.NGSetup()
	if err != nil {
		t.Fatalf("ngRan [id:%d,name:%s,%s:%d] failed to connect AMF, because of [%v]\n", ngRan.N2Cfg.GNBId, ngRan.N2Cfg.GNBName, ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort, err)
	}
	t.Logf("ngRan[id:%d,name:%s,%s:%d] connected to AMF [%s:%d]\n--- ngRan ---\n%s\n", ngRan.N2Cfg.GNBId, ngRan.N2Cfg.GNBName, ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort, ngRan.N2Cfg.AmfIp, ngRan.N2Cfg.AmfPort,ngRan.String())

	t.Log("Starting Ue registration with imsi ...")
	startTime := time.Now()
	for i := 0; i < UeNumberPerNgRan; i++ {
		supi := test.NewSupi()
		if err = ngRan.Registration(supi); err != nil {
			t.Fatalf("ue[%s] failed to registration to AMF, because of %v\n", supi, err)
		}
		t.Logf("UE[%s] succeed to registrate to AMF\n", supi)
	}
	t.Log("--- ngRan ---\n", ngRan.String())

	time.Sleep(time.Millisecond * 100)

	t.Log("Starting Ue deRegistration ...")
	for supi, _ := range ngRan.UeContexts {
		if err = ngRan.DeRegistration(supi); err != nil {
			t.Fatalf("failed de-registration ue[%s], because of %v\n", supi, err)
		}
		t.Logf("UE[%s] succeed to de-registration to AMF\n", supi)
	}

	time.Sleep(time.Millisecond*100)

	if err = ngRan.NGReset(); err != nil{
		t.Fatalf("failed to disconnect to AMF, because of %v\n", err)
	}
	t.Log("succeed to disconnect with AMF")
	t.Log("--- ngRan ---\n", ngRan.String())
	t.Logf("escape time:%dms for %d times Registration and Deregistration\n", time.Since(startTime).Milliseconds(), UeNumberPerNgRan)
}

func TestPduSessionEstablishmentAndRelease(t *testing.T) {
	ngRan := test.NewNgRan()
	err := ngRan.NGSetup()
	if err != nil {
		t.Fatalf("ngRan [id:%d,name:%s,%s:%d] failed to connect AMF, because of [%v]\n", ngRan.N2Cfg.GNBId, ngRan.N2Cfg.GNBName, ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort, err)
	}
	t.Logf("ngRan[id:%d,name:%s,%s:%d] connected to AMF [%s:%d]\n", ngRan.N2Cfg.GNBId, ngRan.N2Cfg.GNBName, ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort, ngRan.N2Cfg.AmfIp, ngRan.N2Cfg.AmfPort)

	startTime := time.Now()
	t.Log("Starting Ue registration with imsi, and establish data pdu session ...")
	for i := 0; i < UeNumberPerNgRan; i++ {
		supi := test.NewSupi()
		if err = ngRan.Registration(supi); err != nil {
			t.Fatalf("ue[%s] failed to registration to AMF, because of %v\n", supi, err)
		}
		if err = ngRan.EstablishPduSession(supi, test.DataPduSessionId, "internet"); err != nil {
			t.Fatalf("ue [%s] failed to establish data pdu session, because of %v\n", supi, err)
		}
		t.Logf("ue[%s] establish pduSession[10,internet]\n", supi)
	}

	t.Log("Starting Ue deRegistration ...")
	for supi, _ := range ngRan.UeContexts {
		if err = ngRan.DeRegistration(supi); err != nil {
			t.Logf("failed de-registration ue[%s], because of %v\n", supi, err)
		}
	}
	t.Logf("escape time:%dms for %d times Registration and Deregistration\n", time.Since(startTime).Milliseconds(), UeNumberPerNgRan)

	time.Sleep(time.Second)

	_ = ngRan.NGReset()
}

func TestAllProcedure(t *testing.T){
	ngRan := test.NewNgRan()
	err := ngRan.NGSetup()
	if err != nil {
		t.Fatalf("ngRan [id:%d,name:%s,%s:%d] failed to connect AMF, because of [%v]\n", ngRan.N2Cfg.GNBId, ngRan.N2Cfg.GNBName, ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort, err)
	}
	t.Logf("ngRan[id:%d,name:%s,%s:%d] connected to AMF [%s:%d]\n", ngRan.N2Cfg.GNBId, ngRan.N2Cfg.GNBName, ngRan.N2Cfg.NgRanIp, ngRan.N2Cfg.NgRanPort, ngRan.N2Cfg.AmfIp, ngRan.N2Cfg.AmfPort)

	startTime := time.Now()
	for i := 0; i < UeNumberPerNgRan; i++ {
		supi := test.NewSupi()
		if err = ngRan.Registration(supi); err != nil {
			t.Fatalf("UE [%s] failed to registration to AMF, because of %v\n", supi, err)
		}
		t.Logf("UE [%s] register to AMF[5g-guti:%s,RanUeNgapId:%d,AmfUeNgapId:%d]\n",supi,ngRan.UeContexts[supi].NasUeCtx.Guti5G,ngRan.UeContexts[supi].RanUeCtx.RanUeNgapId,ngRan.UeContexts[supi].RanUeCtx.AmfUeNgapId)
		time.Sleep(time.Millisecond)
		if err = ngRan.EstablishPduSession(supi, test.DataPduSessionId, "internet"); err != nil {
			t.Fatalf("UE [%s] failed to establish data pdu session, because of %v\n", supi, err)
		}
		t.Logf("UE [%s] establish pduSession[10,internet]\n", supi)
		time.Sleep(time.Millisecond)
		if err = ngRan.N2Release(supi); err != nil{
			t.Fatalf("UE [%s] failed to release N2 connection, because of %v\n", supi, err)
		}
		t.Logf("UE [%s] release N2 connection, and be in idle status\n", supi)
		time.Sleep(time.Millisecond)
		if err = ngRan.ServiceRequest(supi, test.DataPduSessionId); err != nil{
			t.Fatalf("UE [%s] failed to initiate service request, because of %v\n", supi, err)
		}
		t.Logf("UE [%s] initiate service request, and be in activated status\n",supi)
		time.Sleep(time.Millisecond)
		if err = ngRan.DeRegistration(supi); err != nil{
			t.Fatalf("UE [%s] failed to de-registration, becasue of %v\n", supi, err)
		}
		t.Logf("UE [%s] de-registration\n", supi)
		time.Sleep(time.Millisecond)
	}
	_ = ngRan.NGReset()
	t.Logf("escape time:%dms for %d times Registration and Deregistration\n", time.Since(startTime).Milliseconds(), UeNumberPerNgRan)
}
