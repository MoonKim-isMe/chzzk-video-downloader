package chzzk

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "testing"
)
func TestLiveStatusParsing(t *testing.T){
 const channel="0123456789abcdef0123456789abcdef"
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path!="/service/v3/channels/"+channel+"/live-detail"{t.Errorf("unexpected API path %s",r.URL.Path)}
  fmt.Fprint(w,`{"code":200,"content":{"status":"OPEN","liveId":55,"liveTitle":"테스트 방송","openDate":"2026-10-22 10:00:00","channel":{"channelName":"방송인"}}}`)
 }))
 defer server.Close()
 client:=newClient(server.URL,server.Client())
 v,err:=client.GetLiveStatusWithCookiesFile(context.Background(),channel,"")
 if err!=nil{t.Fatal(err)}
 if !v.IsLive||v.LiveID!=55||v.ChannelName!="방송인"{t.Fatalf("unexpected status: %+v",v)}
 _,err=client.GetLiveStatusWithCookiesFile(context.Background(),"bad","")
 if err==nil{t.Fatal("invalid channel accepted")}
}
