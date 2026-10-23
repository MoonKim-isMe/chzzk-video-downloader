// Package liverecording implements a polling-based recorder completely separate
// from the finite VOD download queue. No network calls are made under its mutex.
package liverecording

import (
 "bufio"
 "context"
 "encoding/csv"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "sort"
 "strconv"
 "strings"
 "sync"
 "time"

 "github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
)

type Tools struct { YTDLP string; FFmpeg string }
type Options struct { Directory string; CookiesFile string }
type StatusFunc func(context.Context,string,string)(chzzk.LiveStatus,error)
type ToolsFunc func(context.Context)(Tools,error)
type OptionsFunc func()(Options,error)

type Subscription struct {
 ChannelID string `json:"channelId"`
 ChannelName string `json:"channelName"`
 Enabled bool `json:"enabled"`
 LastError string `json:"lastError,omitempty"`
}
type Session struct {
 ID string `json:"id"`
 ChannelID string `json:"channelId"`
 ChannelName string `json:"channelName"`
 LiveID int64 `json:"liveId"`
 Title string `json:"title"`
 ThumbnailURL string `json:"thumbnailUrl,omitempty"`
 BroadcastStartedAt string `json:"broadcastStartedAt,omitempty"`
 RecordingEndedAt string `json:"recordingEndedAt,omitempty"`
 Directory string `json:"directory"`
 Status string `json:"status"`
 StartedAt string `json:"startedAt"`
 FinishedAt string `json:"finishedAt,omitempty"`
 FinalPath string `json:"finalPath,omitempty"`
 Error string `json:"error,omitempty"`
 AvailableSeconds float64 `json:"availableSeconds"`
}
type Clip struct {
 ID string `json:"id"`
 SessionID string `json:"sessionId"`
 Title string `json:"title"`
 StartSeconds float64 `json:"startSeconds"`
 EndSeconds float64 `json:"endSeconds"`
 Status string `json:"status"`
 FilePath string `json:"filePath,omitempty"`
 Error string `json:"error,omitempty"`
}
type State struct {
 PollSeconds int `json:"pollSeconds"`
 MaxConcurrent int `json:"maxConcurrent"`
 Subscriptions []Subscription `json:"subscriptions"`
 Sessions []Session `json:"sessions"`
 Clips []Clip `json:"clips"`
}
type Service struct {
 mu sync.Mutex
 state State
 path string
 ctx context.Context
 cancel context.CancelFunc
 wake chan struct{}
 status StatusFunc
 tools ToolsFunc
 options OptionsFunc
 active map[string]context.CancelFunc
 clipSlots chan struct{}
 closedCounts map[string]int
 wg sync.WaitGroup
}
var channelPattern=regexp.MustCompile(`^[a-f0-9]{32}$`)
var unsafeName=regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func New(path string, status StatusFunc, tools ToolsFunc, options OptionsFunc) (*Service,error) {
 if status==nil || tools==nil || options==nil { return nil,errors.New("라이브 녹화 의존성이 없습니다") }
 s:=&Service{
  path:path,status:status,tools:tools,options:options,
  state:State{PollSeconds:30,MaxConcurrent:2,Subscriptions:[]Subscription{},Sessions:[]Session{},Clips:[]Clip{}},
  active:make(map[string]context.CancelFunc),closedCounts:make(map[string]int),
  wake:make(chan struct{},1),clipSlots:make(chan struct{},2),
 }
 data,err:=os.ReadFile(path)
 if err==nil {
  if err=json.Unmarshal(data,&s.state);err!=nil{return nil,fmt.Errorf("라이브 녹화 설정을 읽을 수 없습니다: %w",err)}
 } else if !os.IsNotExist(err) {return nil,err}
 if s.state.PollSeconds!=15 && s.state.PollSeconds!=30 && s.state.PollSeconds!=60 {s.state.PollSeconds=30}
 if s.state.MaxConcurrent<1 || s.state.MaxConcurrent>4 {s.state.MaxConcurrent=2}
 if s.state.Subscriptions==nil {s.state.Subscriptions=[]Subscription{}}
 if s.state.Sessions==nil {s.state.Sessions=[]Session{}}
 if s.state.Clips==nil {s.state.Clips=[]Clip{}}
 for i:=range s.state.Sessions {
  if s.state.Sessions[i].Status=="recording" || s.state.Sessions[i].Status=="finalizing" {
   s.state.Sessions[i].Status="interrupted"
   s.state.Sessions[i].Error="앱 종료로 녹화가 중단되었습니다. 기존 분할 파일은 보존됩니다."
  }
 }
 for i:=range s.state.Clips {
  if s.state.Clips[i].Status=="running" || s.state.Clips[i].Status=="queued" {
   s.state.Clips[i].Status="failed";s.state.Clips[i].Error="앱 종료로 구간 추출이 중단되었습니다."
  }
 }
 return s,nil
}
func (s *Service) Start(parent context.Context) {
 s.mu.Lock()
 if s.cancel!=nil {s.mu.Unlock();return}
 s.ctx,s.cancel=context.WithCancel(parent)
 s.wg.Add(1)
 s.mu.Unlock()
 go func(){defer s.wg.Done();s.loop()}()
}
func (s *Service) Stop(){
 s.mu.Lock()
 if s.cancel==nil {s.mu.Unlock();return}
 s.cancel()
 for _,cancel:=range s.active {cancel()}
 s.mu.Unlock()
 s.wg.Wait()
 s.mu.Lock();_ = s.saveLocked();s.mu.Unlock()
}
func (s *Service) Snapshot() State {
 s.mu.Lock();defer s.mu.Unlock()
 result:=s.state
 result.Subscriptions=append([]Subscription{},s.state.Subscriptions...)
 result.Sessions=append([]Session{},s.state.Sessions...)
 result.Clips=append([]Clip{},s.state.Clips...)
 for i:=range result.Sessions{
  if result.Sessions[i].Status=="recording" {
   result.Sessions[i].AvailableSeconds=availableDuration(result.Sessions[i].Directory,true)
  }
 }
 return result
}
func (s *Service) Configure(pollSeconds,maxConcurrent int) (State,error){
 if pollSeconds!=15 && pollSeconds!=30 && pollSeconds!=60 {return State{},errors.New("확인 간격은 15·30·60초 중 선택해야 합니다")}
 if maxConcurrent<1 || maxConcurrent>4 {return State{},errors.New("동시 라이브 녹화 수는 1~4개여야 합니다")}
 s.mu.Lock();s.state.PollSeconds=pollSeconds;s.state.MaxConcurrent=maxConcurrent
 err:=s.saveLocked();s.mu.Unlock();s.notify()
 return s.Snapshot(),err
}
func (s *Service) Watch(channelID,channelName string,enabled bool) (State,error){
 channelID=strings.ToLower(strings.TrimSpace(channelID))
 if !channelPattern.MatchString(channelID){return State{},errors.New("유효한 치지직 채널 ID가 필요합니다")}
 s.mu.Lock()
 found:=false
 for i:=range s.state.Subscriptions{
  if s.state.Subscriptions[i].ChannelID==channelID {
   s.state.Subscriptions[i].Enabled=enabled
   if strings.TrimSpace(channelName)!="" {s.state.Subscriptions[i].ChannelName=strings.TrimSpace(channelName)}
   s.state.Subscriptions[i].LastError=""
   found=true;break
  }
 }
 if !found {s.state.Subscriptions=append(s.state.Subscriptions,Subscription{ChannelID:channelID,ChannelName:strings.TrimSpace(channelName),Enabled:enabled})}
 err:=s.saveLocked();s.mu.Unlock();s.notify()
 return s.Snapshot(),err
}
func (s *Service) Unwatch(channelID string) (State,error){
 s.mu.Lock()
 for i,v:=range s.state.Subscriptions {if v.ChannelID==channelID{s.state.Subscriptions=append(s.state.Subscriptions[:i],s.state.Subscriptions[i+1:]...);break}}
 err:=s.saveLocked();s.mu.Unlock()
 return s.Snapshot(),err
}
func(s *Service) notify(){select{case s.wake<-struct{}{}:default:}}
func(s *Service) loop(){
 for {
  s.check()
  s.mu.Lock();delay:=time.Duration(s.state.PollSeconds)*time.Second;s.mu.Unlock()
  timer:=time.NewTimer(delay)
  select{
   case <-s.ctx.Done():timer.Stop();return
   case <-s.wake:timer.Stop()
   case <-timer.C:
  }
 }
}
func(s *Service) check(){
 s.mu.Lock()
 subs:=append([]Subscription{},s.state.Subscriptions...)
 s.mu.Unlock()
 for _,sub:=range subs{
  if !sub.Enabled {continue}
  select{case <-s.ctx.Done():return;default:}
  options,err:=s.options()
  if err!=nil {s.setError(sub.ChannelID,err);continue}
  ctx,cancel:=context.WithTimeout(s.ctx,12*time.Second)
  live,err:=s.status(ctx,sub.ChannelID,options.CookiesFile)
  cancel()
  if err!=nil {s.setError(sub.ChannelID,err);continue}
  s.mu.Lock()
  // Subscription might be disabled during the network call.
  watching:=false
  for i:=range s.state.Subscriptions {
   if s.state.Subscriptions[i].ChannelID==sub.ChannelID {
    watching=s.state.Subscriptions[i].Enabled
    s.state.Subscriptions[i].LastError=""
    if live.ChannelName!="" {s.state.Subscriptions[i].ChannelName=live.ChannelName}
   }
  }
  if !watching {s.mu.Unlock();continue}
  if _,running:=s.active[sub.ChannelID];running {
   if !live.IsLive {s.closedCounts[sub.ChannelID]++} else {s.closedCounts[sub.ChannelID]=0}
   if s.closedCounts[sub.ChannelID]>=2 {s.active[sub.ChannelID]()}
  } else if live.IsLive && len(s.active)<s.state.MaxConcurrent {
   s.startLocked(live,options,false)
  }
  _=s.saveLocked()
  s.mu.Unlock()
 }
}
func(s *Service) setError(channelID string,err error){
 s.mu.Lock();defer s.mu.Unlock()
 for i:=range s.state.Subscriptions{if s.state.Subscriptions[i].ChannelID==channelID{s.state.Subscriptions[i].LastError=err.Error()}}
 _=s.saveLocked()
}
func(s *Service) RecordNow(channelID string)(Session,error){
 channelID=strings.ToLower(strings.TrimSpace(channelID))
 if !channelPattern.MatchString(channelID){return Session{},errors.New("유효하지 않은 채널 ID입니다")}
 options,err:=s.options();if err!=nil{return Session{},err}
 ctx,cancel:=context.WithTimeout(s.ctx,12*time.Second)
 live,err:=s.status(ctx,channelID,options.CookiesFile);cancel()
 if err!=nil{return Session{},err}
 if !live.IsLive{return Session{},errors.New("현재 진행 중인 라이브 방송이 없습니다")}
 s.mu.Lock();defer s.mu.Unlock()
 if _,ok:=s.active[channelID];ok{return Session{},errors.New("이미 녹화 중인 방송입니다")}
 if len(s.active)>=s.state.MaxConcurrent{return Session{},errors.New("동시 라이브 녹화 한도에 도달했습니다")}
 session,ok:=s.startLocked(live,options,true)
 if !ok{return Session{},errors.New("라이브 녹화를 시작할 수 없습니다")}
 if err:=s.saveLocked();err!=nil{return Session{},err}
 return session,nil
}
// A repeated session requires an explicit user action. Auto watching must
// never restart an already finished recording of this broadcast.
func duplicateBroadcastBlocked(sessions []Session,live chzzk.LiveStatus,manual bool)bool{
 if manual{return false}
 for _,session:=range sessions{
  // Crash-interrupted broadcasts retain the existing automatic resume policy.
  if session.ChannelID==live.ChannelID && session.LiveID==live.LiveID && session.Status!="interrupted"{return true}
 }
 return false
}
func(s *Service) startLocked(live chzzk.LiveStatus,opts Options,manual bool)(Session,bool){
 if live.LiveID<=0 || duplicateBroadcastBlocked(s.state.Sessions,live,manual){return Session{},false}
 stamp:=time.Now().Format("20060102_150405.000")
 folder:=filepath.Join(opts.Directory,"라이브",safeName(live.ChannelName),"live_"+strconv.FormatInt(live.LiveID,10)+"_"+stamp)
 session:=Session{ID:fmt.Sprintf("%s-%d-%d",live.ChannelID,live.LiveID,time.Now().UnixNano()),ChannelID:live.ChannelID,ChannelName:live.ChannelName,LiveID:live.LiveID,Title:live.LiveTitle,ThumbnailURL:live.ThumbnailURL,BroadcastStartedAt:live.OpenDate,Directory:folder,Status:"recording",StartedAt:time.Now().Format(time.RFC3339)}
 ctx,cancel:=context.WithCancel(s.ctx);s.active[live.ChannelID]=cancel;s.closedCounts[live.ChannelID]=0
 s.state.Sessions=append([]Session{session},s.state.Sessions...)
 s.wg.Add(1)
 go func(){defer s.wg.Done();s.record(ctx,session.ID,opts)}()
 return session,true
}
func safeName(name string)string{
 name=strings.Trim(strings.TrimSpace(unsafeName.ReplaceAllString(name,"_")),". ")
 if name=="" {name="채널"}
 r:=[]rune(name);if len(r)>80 {name=string(r[:80])}
 return name
}
func(s *Service) findSessionLocked(id string)*Session{
 for i:=range s.state.Sessions {if s.state.Sessions[i].ID==id{return &s.state.Sessions[i]}}
 return nil
}
func(s *Service) StopRecording(sessionID string)bool{
 s.mu.Lock();defer s.mu.Unlock()
 current:=s.findSessionLocked(sessionID)
 if current==nil || current.Status!="recording"{return false}
 cancel,ok:=s.active[current.ChannelID]
 if ok{cancel()}
 return ok
}
func(s *Service) record(ctx context.Context,id string,opts Options){
 s.mu.Lock();session:=s.findSessionLocked(id);if session==nil{s.mu.Unlock();return}
 channelID:=session.ChannelID;folder:=session.Directory;s.mu.Unlock()
 failure:=error(nil)
 if err:=os.MkdirAll(folder,0755);err!=nil{failure=err}
 // Each retry receives a fresh signed stream URL. Previously completed
 // MPEG-TS segments are never overwritten, even after an interrupted FFmpeg.
 for attempt:=0;failure==nil && attempt<12;attempt++{
  if ctx.Err()!=nil{break}
  tools,err:=s.tools(ctx);if err!=nil{failure=err;break}
  url,err:=s.resolveStream(ctx,tools.YTDLP,channelID,opts.CookiesFile)
  if err!=nil{
   if !sleep(ctx,5*time.Second){break}
   continue
  }
  startNumber:=nextSegmentNumber(folder)
  playlist:=filepath.Join(folder,fmt.Sprintf("index_%06d.csv",startNumber))
  args:=[]string{"-hide_banner","-loglevel","error","-nostdin","-i",url,"-map","0:v:0","-map","0:a?","-c","copy","-f","segment","-segment_time","30","-segment_format","mpegts","-segment_list_type","csv","-segment_list",playlist,"-segment_start_number",strconv.Itoa(startNumber),filepath.Join(folder,"segment_%06d.ts")}
  cmd:=exec.CommandContext(ctx,tools.FFmpeg,args...);hideWindow(cmd)
  // Signed playback URLs may occur in ffmpeg stderr; never store raw output.
  err=cmd.Run()
  if ctx.Err()!=nil {break}
  statusCtx,cancel:=context.WithTimeout(ctx,12*time.Second)
  current,statusErr:=s.status(statusCtx,channelID,opts.CookiesFile);cancel()
  if statusErr==nil && !current.IsLive{break}
  if err!=nil && attempt==11{failure=errors.New("라이브 스트림이 반복적으로 중단되었습니다")}
  if !sleep(ctx,5*time.Second){break}
 }
 // The recorder's cancellation is intentional during StopRecording or an
 // application shutdown; all completed segments remain available.
 s.mu.Lock();current:=s.findSessionLocked(id)
 if current!=nil{current.Status="finalizing";current.RecordingEndedAt=time.Now().Format(time.RFC3339);current.AvailableSeconds=availableDuration(folder,false)}
 _=s.saveLocked();s.mu.Unlock()
 files,_:=segmentFiles(folder,false)
 finalPath:=""
 if len(files)>0{
  tools,err:=s.tools(context.Background())
  if err==nil {
   finalPath=filepath.Join(folder,"전체 녹화.mp4")
   mergeCtx,mergeCancel:=context.WithTimeout(context.Background(),2*time.Minute)
   err=concatFiles(mergeCtx,tools.FFmpeg,folder,files,finalPath,0,0,false)
   mergeCancel()
   if err!=nil{finalPath="";failure=errors.New("전체 녹화 MP4 생성에 실패했습니다. 원본 분할 파일은 보존됩니다")}
  } else{failure=err}
 }else if failure==nil{failure=errors.New("저장된 영상 구간이 없습니다")}
 s.mu.Lock()
 if current=s.findSessionLocked(id);current!=nil{
  current.FinishedAt=time.Now().Format(time.RFC3339)
  current.FinalPath=finalPath
  if failure!=nil{current.Status="failed";current.Error=failure.Error()} else{current.Status="completed"}
  current.AvailableSeconds=availableDuration(folder,false)
 }
 delete(s.active,channelID);delete(s.closedCounts,channelID)
 _=s.saveLocked();s.mu.Unlock()
 s.notify()
}
func sleep(ctx context.Context,d time.Duration)bool{
 t:=time.NewTimer(d);defer t.Stop()
 select{case <-ctx.Done():return false;case <-t.C:return true}
}
func(s *Service) resolveStream(ctx context.Context,ytDLP,channelID,cookieFile string)(string,error){
 args:=[]string{"--no-playlist","--no-warnings","-f","b","-g"}
 if cookieFile!=""{args=append(args,"--cookies",cookieFile)}
 args=append(args,"https://chzzk.naver.com/live/"+channelID)
 probe,cancel:=context.WithTimeout(ctx,25*time.Second);defer cancel()
 cmd:=exec.CommandContext(probe,ytDLP,args...);hideWindow(cmd)
 out,err:=cmd.Output()
 if err!=nil{return "",errors.New("라이브 재생 주소를 확인할 수 없습니다")}
 candidate:=strings.TrimSpace(strings.SplitN(string(out),"\n",2)[0])
 if !strings.HasPrefix(candidate,"https://"){return "",errors.New("라이브 재생 주소가 올바르지 않습니다")}
 return candidate,nil
}
func nextSegmentNumber(folder string)int{
 files,_:=filepath.Glob(filepath.Join(folder,"segment_*.ts"));max:=-1
 for _,file:=range files{var n int;if _,err:=fmt.Sscanf(filepath.Base(file),"segment_%06d.ts",&n);err==nil && n>max{max=n}}
 return max+1
}
type segmentFile struct{Path string;Duration float64}
func segmentFiles(folder string,isRecording bool)([]segmentFile,error){
 lists,err:=filepath.Glob(filepath.Join(folder,"index_*.csv"));if err!=nil{return nil,err}
 sort.Strings(lists)
 result:=[]segmentFile{}
 seen:=map[string]bool{}
 for _,list:=range lists{
  f,err:=os.Open(list);if err!=nil{continue}
  rd:=csv.NewReader(bufio.NewReader(f));rd.FieldsPerRecord=-1
  for{
   row,err:=rd.Read()
   if err!=nil{break}
   if len(row)<3{continue}
   start,e1:=strconv.ParseFloat(row[1],64);end,e2:=strconv.ParseFloat(row[2],64)
   if e1!=nil || e2!=nil || end<=start {continue}
   name:=filepath.Base(row[0])
   if !regexp.MustCompile(`^segment_[0-9]{6}\.ts$`).MatchString(name){continue}
   if seen[name]{continue}
   path:=filepath.Join(folder,name)
   if info,err:=os.Stat(path);err!=nil || info.Size()==0{continue}
   seen[name]=true
   result=append(result,segmentFile{path,end-start})
  }
  f.Close()
 }
 // While ffmpeg is still running, the highest numbered segment may be
 // receiving writes. Exclude it until the recorder has closed it.
 if isRecording && len(result)>0{
  newest:=nextSegmentNumber(folder)-1
  name:=fmt.Sprintf("segment_%06d.ts",newest)
  filtered:=result[:0]
  for _,f:=range result{if filepath.Base(f.Path)!=name{filtered=append(filtered,f)}}
  result=filtered
 }
 return result,nil
}
func availableDuration(folder string,recording bool)float64{
 files,_:=segmentFiles(folder,recording);var n float64
 for _,file:=range files{n+=file.Duration}
 return n
}
func concatFiles(ctx context.Context,ffmpeg,folder string,files []segmentFile,out string,start,duration float64,exact bool)error{
 if len(files)==0{return errors.New("선택된 녹화 조각이 없습니다")}
 list,err:=os.CreateTemp(folder,"concat-*.txt");if err!=nil{return err}
 defer os.Remove(list.Name())
 for _,file:=range files{
  // ffmpeg concat demuxer supports escaped absolute file paths.
  path:=strings.ReplaceAll(file.Path,"'","'\\''")
  if _,err:=fmt.Fprintf(list,"file '%s'\n",path);err!=nil{list.Close();return err}
 }
 if err=list.Close();err!=nil{return err}
 temporary:=out+".partial.mp4";defer os.Remove(temporary)
 args:=[]string{"-hide_banner","-loglevel","error","-nostdin","-y","-f","concat","-safe","0","-i",list.Name()}
 if start>0 {args=append(args,"-ss",strconv.FormatFloat(start,'f',3,64))}
 if duration>0 {args=append(args,"-t",strconv.FormatFloat(duration,'f',3,64))}
 if exact {args=append(args,"-c:v","libx264","-preset","veryfast","-crf","19","-c:a","aac")}else{args=append(args,"-c","copy")}
 args=append(args,"-movflags","+faststart",temporary)
 cmd:=exec.CommandContext(ctx,ffmpeg,args...);hideWindow(cmd)
 if err=cmd.Run();err!=nil{return errors.New("FFmpeg 영상 파일 생성에 실패했습니다")}
 return os.Rename(temporary,out)
}
func(s *Service) ExtractClip(sessionID,title string,start,end float64,exact bool)(Clip,error){
 s.mu.Lock()
 session:=s.findSessionLocked(sessionID)
 if session==nil{s.mu.Unlock();return Clip{},errors.New("녹화 기록이 없습니다")}
 if start<0 || end<=start{s.mu.Unlock();return Clip{},errors.New("구간 시작·종료 시간을 확인해 주세요")}
 isRecording:=session.Status=="recording"
 files,err:=segmentFiles(session.Directory,isRecording)
 if err!=nil{s.mu.Unlock();return Clip{},err}
 var total float64
 for _,file:=range files{total+=file.Duration}
 if end>total+0.01{s.mu.Unlock();return Clip{},fmt.Errorf("저장 완료된 구간은 %.1f초까지입니다",total)}
 clip:=Clip{ID:fmt.Sprintf("clip-%d",time.Now().UnixNano()),SessionID:session.ID,Title:title,StartSeconds:start,EndSeconds:end,Status:"queued"}
 s.state.Clips=append([]Clip{clip},s.state.Clips...)
 folder:=session.Directory
 _=s.saveLocked();s.mu.Unlock()
 s.wg.Add(1)
 go func(){defer s.wg.Done();s.extract(clip,folder,files,exact)}()
 return clip,nil
}
func(s *Service) extract(clip Clip,folder string,files []segmentFile,exact bool){
 select{case s.clipSlots<-struct{}{}:case <-s.ctx.Done():return}
 defer func(){<-s.clipSlots}()
 s.clipUpdate(clip.ID,"running","","")
 var at float64;begin:=-1;offset:=0.0;selected:=[]segmentFile{}
 for i,file:=range files{
  if at+file.Duration>clip.StartSeconds && at<clip.EndSeconds {
   if begin<0 {begin=i;offset=clip.StartSeconds-at}
   selected=append(selected,file)
  }
  at+=file.Duration
 }
 if len(selected)==0{s.clipUpdate(clip.ID,"failed","","추출할 저장 구간이 없습니다");return}
 opts,err:=s.tools(s.ctx)
 if err!=nil{s.clipUpdate(clip.ID,"failed","",err.Error());return}
 name:=safeName(clip.Title)
 if name=="채널"{name="구간"}
 out:=filepath.Join(folder,fmt.Sprintf("%s_%s.mp4",name,strings.TrimPrefix(clip.ID,"clip-")))
 err=concatFiles(s.ctx,opts.FFmpeg,folder,selected,out,offset,clip.EndSeconds-clip.StartSeconds,exact)
 if err!=nil{s.clipUpdate(clip.ID,"failed","",err.Error());return}
 s.clipUpdate(clip.ID,"completed",out,"")
}
func(s *Service) clipUpdate(id,status,path,problem string){
 s.mu.Lock();defer s.mu.Unlock()
 for i:=range s.state.Clips{if s.state.Clips[i].ID==id{s.state.Clips[i].Status=status;s.state.Clips[i].FilePath=path;s.state.Clips[i].Error=problem;break}}
 _=s.saveLocked()
}
func(s *Service) saveLocked()error{
 if s.path==""{return errors.New("라이브 상태 저장 경로가 없습니다")}
 if err:=os.MkdirAll(filepath.Dir(s.path),0700);err!=nil{return err}
 raw,err:=json.MarshalIndent(s.state,"","  ");if err!=nil{return err}
 tmp,err:=os.CreateTemp(filepath.Dir(s.path),"live-state-*.json");if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if _,err=tmp.Write(raw);err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 // Windows cannot rename over an existing target; remove only after the
 // new complete snapshot has been written.
 if err=os.Rename(tmp.Name(),s.path);err==nil{return nil}
 // Windows may reject rename over an existing file.
 if _,statErr:=os.Stat(s.path);statErr==nil{
  if removeErr:=os.Remove(s.path);removeErr!=nil{return removeErr}
  return os.Rename(tmp.Name(),s.path)
 }
 return err
}
