package liverecording

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
)

const sampleChannel="0123456789abcdef0123456789abcdef"
func fakeService(t *testing.T)*Service{
 t.Helper()
 root:=t.TempDir()
 s,err:=New(filepath.Join(root,"state.json"),
  func(context.Context,string,string)(chzzk.LiveStatus,error){return chzzk.LiveStatus{ChannelID:sampleChannel,LiveID:123,IsLive:false},nil},
  func(context.Context)(Tools,error){return Tools{},nil},
  func()(Options,error){return Options{Directory:root},nil},
 )
 if err!=nil{t.Fatal(err)}
 return s
}
func TestSubscriptionPersistenceAndIndependentLimit(t *testing.T){
 s:=fakeService(t)
 if _,err:=s.Configure(60,4);err!=nil{t.Fatal(err)}
 if _,err:=s.Watch(sampleChannel,"테스트",true);err!=nil{t.Fatal(err)}
 if _,err:=s.Watch(sampleChannel,"테스트",false);err!=nil{t.Fatal(err)}
 state:=s.Snapshot()
 if state.MaxConcurrent!=4||state.PollSeconds!=60||len(state.Subscriptions)!=1||state.Subscriptions[0].Enabled{
  t.Fatalf("unexpected state: %+v",state)
 }
 restored,err:=New(s.path,s.status,s.tools,s.options)
 if err!=nil{t.Fatal(err)}
 if len(restored.Snapshot().Subscriptions)!=1||restored.Snapshot().MaxConcurrent!=4{t.Fatalf("restored: %+v",restored.Snapshot())}
 if _,err=restored.Configure(1,3);err==nil{t.Fatal("invalid polling interval accepted")}
 if _,err=restored.Configure(30,5);err==nil{t.Fatal("invalid concurrency accepted")}
 if _,err=restored.Watch("invalid","",true);err==nil{t.Fatal("invalid channel accepted")}
}
func TestCompletedSegmentOnlyDuration(t *testing.T){
 folder:=t.TempDir()
 files:=[]string{"segment_000000.ts","segment_000001.ts"}
 for _,name:=range files{if err:=os.WriteFile(filepath.Join(folder,name),[]byte("data"),0600);err!=nil{t.Fatal(err)}}
 csvData:="segment_000000.ts,0,30\nsegment_000001.ts,30,60\n"
 if err:=os.WriteFile(filepath.Join(folder,"index_000000.csv"),[]byte(csvData),0600);err!=nil{t.Fatal(err)}
 recorded,err:=segmentFiles(folder,true);if err!=nil{t.Fatal(err)}
 if len(recorded)!=1||recorded[0].Duration!=30{t.Fatalf("in-progress segments included: %+v",recorded)}
 completed,err:=segmentFiles(folder,false);if err!=nil{t.Fatal(err)}
 if len(completed)!=2||availableDuration(folder,false)!=60{t.Fatalf("completed segment duration incorrect: %+v",completed)}
}
func TestRunningSessionRecoveryPreservesFiles(t *testing.T){
 s:=fakeService(t)
 s.state.Sessions=append(s.state.Sessions,Session{ID:"example",ChannelID:sampleChannel,LiveID:23,Status:"recording"})
 if err:=s.saveLocked();err!=nil{t.Fatal(err)}
 restored,err:=New(s.path,s.status,s.tools,s.options);if err!=nil{t.Fatal(err)}
 if restored.Snapshot().Sessions[0].Status!="interrupted"{t.Fatal("unfinished session was not recovered")}
}
func TestSubscriptionLoopDoesNotRecordOffline(t *testing.T){
 s:=fakeService(t)
 if _,err:=s.Watch(sampleChannel,"테스트",true);err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 s.Start(ctx)
 time.Sleep(40*time.Millisecond)
 cancel();s.Stop()
 if len(s.Snapshot().Sessions)!=0{t.Fatal("offline channel recorded")}
}
func TestSafeName(t *testing.T){
 if strings.ContainsAny(safeName("bad:/\\*name"),":/\\*"){t.Fatal("unsafe name")}
}

func TestSessionBroadcastMetadataPersistence(t *testing.T) {
 s:=fakeService(t)
 session:=Session{ID:"record-test",ChannelID:sampleChannel,LiveID:12,Title:"방송 제목",ThumbnailURL:"https://example.com/thumbnail.jpg",BroadcastStartedAt:"2026-10-23 12:00:00",Tags:[]string{"게임","챌린지"},LiveCategoryValue:"엘든 링",StartedAt:"2026-10-23T12:01:00+09:00",RecordingEndedAt:"2026-10-23T12:05:00+09:00",Status:"completed"}
 s.state.Sessions=append(s.state.Sessions,session)
 if err:=s.saveLocked();err!=nil{t.Fatal(err)}
 restored,err:=New(s.path,s.status,s.tools,s.options)
 if err!=nil{t.Fatal(err)}
 got:=restored.Snapshot().Sessions[0]
 if got.ThumbnailURL!=session.ThumbnailURL || got.BroadcastStartedAt!=session.BroadcastStartedAt || got.RecordingEndedAt!=session.RecordingEndedAt || got.LiveCategoryValue!=session.LiveCategoryValue || len(got.Tags)!=2 || got.Tags[0]!="게임" {
  t.Fatalf("session metadata was not persisted: %+v",got)
 }
}

func TestManualRestartSameLiveAfterFinishedSession(t *testing.T) {
 live:=chzzk.LiveStatus{ChannelID:sampleChannel,LiveID:123,IsLive:true}
 for _,status:=range []string{"completed","failed","interrupted"} {
  t.Run(status,func(t *testing.T){
   history:=[]Session{{ChannelID:sampleChannel,LiveID:live.LiveID,Status:status}}
   if duplicateBroadcastBlocked(history,live,true){t.Fatal("explicit manual restart was blocked")}
   if status!="interrupted" && !duplicateBroadcastBlocked(history,live,false){t.Fatal("finished broadcast was restarted automatically")}
   if status=="interrupted" && duplicateBroadcastBlocked(history,live,false){t.Fatal("automatic recovery of interrupted broadcast was blocked")}
  })
 }
}

func TestAutomaticRecordingIgnoresOldBroadcastButPreventsRepeatedLiveID(t *testing.T) {
 live:=chzzk.LiveStatus{ChannelID:sampleChannel,LiveID:123,IsLive:true}
 history:=[]Session{
  {ChannelID:sampleChannel,LiveID:live.LiveID,Status:"completed"},
  {ChannelID:sampleChannel,LiveID:live.LiveID,Status:"failed"},
 }
 if !duplicateBroadcastBlocked(history,live,false){t.Fatal("automatic recording repeated same broadcast")}
 if duplicateBroadcastBlocked(history,chzzk.LiveStatus{ChannelID:sampleChannel,LiveID:124},false){t.Fatal("new broadcast ID blocked")}
 if duplicateBroadcastBlocked(history,chzzk.LiveStatus{ChannelID:"11111111111111111111111111111111",LiveID:live.LiveID},false){t.Fatal("other channel blocked")}
 if duplicateBroadcastBlocked(nil,live,false){t.Fatal("first-time broadcast blocked")}
}

func TestRecordNowRestartsFinishedBroadcastWithIndependentSession(t *testing.T) {
 s:=fakeService(t)
 s.status=func(context.Context,string,string)(chzzk.LiveStatus,error){
  return chzzk.LiveStatus{ChannelID:sampleChannel,ChannelName:"테스트",LiveID:123,IsLive:true},nil
 }
 // The canceled recorder context makes both jobs finalize immediately, without
 // external FFmpeg/yt-dlp processes, while RecordNow still verifies live status.
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 s.ctx=ctx
 first,err:=s.RecordNow(sampleChannel)
 if err!=nil{t.Fatalf("first manual recording failed: %v",err)}
 s.wg.Wait()
 second,err:=s.RecordNow(sampleChannel)
 if err!=nil{t.Fatalf("manual restart in same live failed: %v",err)}
 s.wg.Wait()
 if first.ID==second.ID || first.Directory==second.Directory {
  t.Fatal("restarted recording reused the previous session identity or directory")
 }
 state:=s.Snapshot()
 if len(state.Sessions)!=2 || state.Sessions[0].ID!=second.ID || state.Sessions[1].ID!=first.ID {
  t.Fatalf("recording history not preserved: %+v",state.Sessions)
 }
 if !duplicateBroadcastBlocked(state.Sessions,chzzk.LiveStatus{ChannelID:sampleChannel,LiveID:123},false){
  t.Fatal("automatic watcher should not restart the same finished live")
 }
}
