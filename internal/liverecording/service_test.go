package liverecording

import (
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
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

func TestDeleteRecordingHistoryPreservesFilesAndAutoDedup(t *testing.T){
 s:=fakeService(t)
 folder:=filepath.Join(t.TempDir(),"recording")
 if err:=os.MkdirAll(folder,0755);err!=nil{t.Fatal(err)}
 videoPath:=filepath.Join(folder,"전체 녹화.mp4")
 if err:=os.WriteFile(videoPath,[]byte("file contents"),0600);err!=nil{t.Fatal(err)}
 s.state.Sessions=[]Session{
  {ID:"done",ChannelID:sampleChannel,LiveID:123,Directory:folder,Status:"completed"},
  {ID:"ongoing",ChannelID:sampleChannel,LiveID:124,Status:"recording"},
 }
 s.state.Clips=[]Clip{{ID:"clip1",SessionID:"done",Status:"completed"},{ID:"clip2",SessionID:"ongoing",Status:"queued"}}
 if err:=s.saveLocked();err!=nil{t.Fatal(err)}
 if _,err:=s.DeleteHistory("ongoing");err==nil{t.Fatal("ongoing recording deleted")}
 state,err:=s.DeleteHistory("done");if err!=nil{t.Fatal(err)}
 if len(state.Sessions)!=1||state.Sessions[0].ID!="ongoing"{t.Fatalf("wrong history: %+v",state.Sessions)}
 if len(state.Clips)!=1||state.Clips[0].ID!="clip2"{t.Fatalf("wrong clip history: %+v",state.Clips)}
 if _,err=os.Stat(videoPath);err!=nil{t.Fatalf("recorded video was removed: %v",err)}
 live:=chzzk.LiveStatus{ChannelID:sampleChannel,LiveID:123}
 if !recordedBroadcastBlocked(state.RecordedBroadcasts,live){t.Fatal("removed history restarted in auto watch")}
 restored,err:=New(s.path,s.status,s.tools,s.options);if err!=nil{t.Fatal(err)}
 if len(restored.Snapshot().Sessions)!=1 || !recordedBroadcastBlocked(restored.Snapshot().RecordedBroadcasts,live){t.Fatal("deletion or dedupe was not persisted")}
 if _,err=restored.DeleteHistory("done");err==nil{t.Fatal("missing history accepted")}
}

func TestClearRecordingHistoryKeepsActiveSessions(t *testing.T){
 s:=fakeService(t)
 s.state.Sessions=[]Session{
  {ID:"done1",ChannelID:sampleChannel,LiveID:200,Status:"completed"},
  {ID:"done2",ChannelID:sampleChannel,LiveID:200,Status:"failed"},
  {ID:"interrupted",ChannelID:sampleChannel,LiveID:201,Status:"interrupted"},
  {ID:"recording",ChannelID:sampleChannel,LiveID:202,Status:"recording"},
  {ID:"finalizing",ChannelID:sampleChannel,LiveID:203,Status:"finalizing"},
 }
 s.state.Clips=[]Clip{
  {ID:"clip1",SessionID:"done1",Status:"completed"},
  {ID:"clip2",SessionID:"recording",Status:"queued"},
 }
 state,err:=s.ClearHistory();if err!=nil{t.Fatal(err)}
 if len(state.Sessions)!=2 || state.Sessions[0].ID!="recording" || state.Sessions[1].ID!="finalizing"{t.Fatalf("active sessions removed: %+v",state.Sessions)}
 if len(state.Clips)!=1 || state.Clips[0].ID!="clip2"{t.Fatalf("active clip removed: %+v",state.Clips)}
 if len(state.RecordedBroadcasts)!=1 || state.RecordedBroadcasts[0].LiveID!=200{t.Fatalf("dedupe markers: %+v",state.RecordedBroadcasts)}
 if recordedBroadcastBlocked(state.RecordedBroadcasts,chzzk.LiveStatus{ChannelID:sampleChannel,LiveID:201}){t.Fatal("interrupted auto restart blocked")}
 if _,err=s.ClearHistory();err!=nil{t.Fatalf("empty cleanup failed: %v",err)}
}

func TestHistoryDeletionRejectsActiveClipAndRollsBackOnSaveError(t *testing.T){
 s:=fakeService(t)
 s.state.Sessions=[]Session{{ID:"done",ChannelID:sampleChannel,LiveID:200,Status:"completed"}}
 s.state.Clips=[]Clip{{ID:"clip",SessionID:"done",Status:"running"}}
 if _,err:=s.DeleteHistory("done");err==nil{t.Fatal("active clip deletion accepted")}
 if _,err:=s.ClearHistory();err==nil{t.Fatal("bulk clear accepted while extracting")}
 s.state.Clips[0].Status="completed"
 original:=s.path
 folder:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(folder,"keep.txt"),[]byte("do not remove"),0600);err!=nil{t.Fatal(err)}
 s.path=folder // replacing a non-empty directory must fail
 if _,err:=s.DeleteHistory("done");err==nil{t.Fatal("save failure not returned")}
 s.path=original
 if len(s.Snapshot().Sessions)!=1 || len(s.Snapshot().Clips)!=1 || len(s.Snapshot().RecordedBroadcasts)!=0{
  t.Fatal("failed save modified in-memory state")
 }
}

func TestRemovedHistoryDoesNotRestartAutoButAllowsManualRecording(t *testing.T){
 s:=fakeService(t)
 live:=chzzk.LiveStatus{ChannelID:sampleChannel,LiveID:55,IsLive:true}
 s.state.Sessions=[]Session{{ID:"old",ChannelID:sampleChannel,LiveID:55,Status:"completed"}}
 if _,err:=s.DeleteHistory("old");err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 s.ctx=ctx
 opts:=Options{Directory:t.TempDir()}
 s.mu.Lock()
 if _,ok:=s.startLocked(live,opts,false);ok {
  s.mu.Unlock()
  t.Fatal("automatic recording ignored deleted-history dedupe marker")
 }
 if _,ok:=s.startLocked(live,opts,true);!ok {
  s.mu.Unlock()
  t.Fatal("manual recording was blocked after deleting history")
 }
 s.mu.Unlock()
 s.wg.Wait()
 if len(s.Snapshot().Sessions)!=1{t.Fatal("manual restart not recorded")}
}

func TestParseFFmpegProgressLine(t *testing.T){
 cases:=[]struct{line string;want float64;valid bool}{
  {"out_time_us=12500000",12.5,true},
  {"out_time_ms=1534000",1.534,true}, // historical name is still microseconds
  {"out_time_us=0",0,true},
  {"out_time_us=-1",0,false},
  {"out_time_us=N/A",0,false},
  {"progress=continue",0,false},
  {"out_time=00:00:01.25",0,false},
  {"out_time_us=999999999999999999999999",0,false},
 }
 for _,tc:=range cases{
  got,ok:=parseFFmpegProgressLine(tc.line)
  if got!=tc.want || ok!=tc.valid{t.Errorf("%s => %v %v, want %v %v",tc.line,got,ok,tc.want,tc.valid)}
 }
}

func TestRecorderProgressIsMonotonicAndDoesNotIncreaseClipAvailability(t *testing.T){
 s:=fakeService(t)
 folder:=t.TempDir()
 s.state.Sessions=[]Session{{ID:"recording",ChannelID:sampleChannel,LiveID:123,Directory:folder,Status:"recording"}}
 s.updateRecorderProgress("recording",0,4.4)
 s.updateRecorderProgress("recording",0,3.0) // FFmpeg duplicate/out-of-order update
 got:=s.Snapshot().Sessions[0]
 if got.RecordedSeconds!=4.4||got.AvailableSeconds!=0{
  t.Fatalf("FFmpeg progress leaked into extractable duration: %+v",got)
 }
 s.updateRecorderProgress("recording",4.4,2.2) // a reconnect resets FFmpeg out_time
 got=s.Snapshot().Sessions[0]
 if got.RecordedSeconds<6.59 || got.RecordedSeconds>6.61{
  t.Fatalf("progress did not accumulate across reconnect: %v",got.RecordedSeconds)
 }
 s.updateRecorderProgress("missing",0,100)
 if s.Snapshot().Sessions[0].RecordedSeconds>6.61{t.Fatal("progress changed for missing session")}
 s.state.Sessions[0].Status="completed"
 s.state.Sessions[0].AvailableSeconds=6
 delete(s.progress,"recording")
 got=s.Snapshot().Sessions[0]
 if got.RecordedSeconds!=0||got.AvailableSeconds!=6{t.Fatalf("finished recording did not use final segment duration: %+v",got)}
}

func TestRecorderProgressHelperProcess(t *testing.T){
 if os.Getenv("CHZZK_FFMPEG_PROGRESS_HELPER")!="1"{return}
 fmt.Println("frame=30")
 fmt.Println("out_time_us=1500000")
 fmt.Println("progress=continue")
 fmt.Println("out_time_us=2500000")
 fmt.Println("progress=end")
 os.Exit(0)
}

func TestFFmpegProgressStdoutReader(t *testing.T){
 s:=fakeService(t)
 s.state.Sessions=[]Session{{ID:"recording",ChannelID:sampleChannel,LiveID:123,Directory:t.TempDir(),Status:"recording"}}
 ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestRecorderProgressHelperProcess$")
 cmd.Env=append(os.Environ(),"CHZZK_FFMPEG_PROGRESS_HELPER=1")
 if err:=s.runRecorderWithProgress(cmd,"recording",30);err!=nil{t.Fatal(err)}
 got:=s.Snapshot().Sessions[0]
 if got.RecordedSeconds!=32.5 || got.AvailableSeconds!=0{
  t.Fatalf("FFmpeg stdout updates not combined with previous segments: %+v",got)
 }
}

func TestRecorderWatchdogHelperProcess(t *testing.T){
 mode:=os.Getenv("CHZZK_FFMPEG_WATCHDOG_HELPER")
 if mode==""{return}
 for i:=0;i<8;i++{
  seconds:=1
  if mode=="catchup"{seconds=i+1}
  fmt.Printf("out_time_us=%d\n",seconds*1_000_000)
  time.Sleep(50*time.Millisecond)
 }
 if mode=="stall"{time.Sleep(2500*time.Millisecond)}
 os.Exit(0)
}

func TestRecorderStallRestartsWhenDuplicateProgressNeverAdvances(t *testing.T){
 s:=fakeService(t)
 s.state.Sessions=[]Session{{ID:"recording",ChannelID:sampleChannel,LiveID:123,Directory:t.TempDir(),Status:"recording"}}
 ctx,cancel:=context.WithTimeout(context.Background(),6*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestRecorderWatchdogHelperProcess$")
 cmd.Env=append(os.Environ(),"CHZZK_FFMPEG_WATCHDOG_HELPER=stall")
 err:=s.runRecorderWithProgressTimeout(cmd,"recording",30,1500*time.Millisecond)
 if !errors.Is(err,errRecorderStalled){t.Fatalf("expected stalled reconnect after duplicate progress, got %v",err)}
 got:=s.Snapshot().Sessions[0]
 if got.RecordedSeconds!=31 || got.AvailableSeconds!=0{
  t.Fatalf("watchdog changed recorded or extractable time: %+v",got)
 }
}

func TestRecorderWatchdogKeepsReceivingDelayedSegments(t *testing.T){
 s:=fakeService(t)
 s.state.Sessions=[]Session{{ID:"recording",ChannelID:sampleChannel,LiveID:123,Directory:t.TempDir(),Status:"recording"}}
 ctx,cancel:=context.WithTimeout(context.Background(),6*time.Second);defer cancel()
 cmd:=exec.CommandContext(ctx,os.Args[0],"-test.run=^TestRecorderWatchdogHelperProcess$")
 cmd.Env=append(os.Environ(),"CHZZK_FFMPEG_WATCHDOG_HELPER=catchup")
 if err:=s.runRecorderWithProgressTimeout(cmd,"recording",30,1500*time.Millisecond);err!=nil{
  t.Fatalf("active catch-up should not be interrupted: %v",err)
 }
 got:=s.Snapshot().Sessions[0]
 if got.RecordedSeconds!=38 || got.AvailableSeconds!=0{
  t.Fatalf("catch-up progress incorrect: %+v",got)
 }
}
