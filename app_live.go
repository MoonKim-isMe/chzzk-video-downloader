package main

import (
 "context"
 "errors"
 "fmt"
 "path/filepath"

 "github.com/MoonKim-isMe/chzzk-video-downloader/internal/chzzk"
 "github.com/MoonKim-isMe/chzzk-video-downloader/internal/liverecording"
 "github.com/MoonKim-isMe/chzzk-video-downloader/internal/persistence"
)

// GetChannelLiveStatus reads the live broadcast without entering the VOD queue.
func (a *App) GetChannelLiveStatus(channelID string) (chzzk.LiveStatus, error) {
	cookies, err := a.activeAuthenticationCookiesFile()
	if err != nil {
		return chzzk.LiveStatus{}, err
	}
	return a.chzzkClient.GetLiveStatusWithCookiesFile(a.appContext(), channelID, cookies)
}
func (a *App) startLiveRecordingService() error {
 a.liveMu.Lock()
 defer a.liveMu.Unlock()
 if a.liveRecorder!=nil {return nil}
 pathFn:=a.databasePath
 if pathFn==nil{pathFn=persistence.DefaultPath}
 dbPath,err:=pathFn()
 if err!=nil{return err}
 recorder,err:=liverecording.New(
  filepath.Join(filepath.Dir(dbPath),"live-recording.json"),
  func(ctx context.Context,id,cookies string)(chzzk.LiveStatus,error){
   return a.chzzkClient.GetLiveStatusWithCookiesFile(ctx,id,cookies)
  },
  func(ctx context.Context)(liverecording.Tools,error){
   tools:=a.downloadManager.ToolchainStatus(ctx)
   if !tools.YTDLP.Available || !tools.FFmpeg.Available {return liverecording.Tools{},errors.New("라이브 녹화에 필요한 yt-dlp 또는 FFmpeg를 사용할 수 없습니다")}
   return liverecording.Tools{YTDLP:tools.YTDLP.Path,FFmpeg:tools.FFmpeg.Path},nil
  },
  func()(liverecording.Options,error){
   settings,err:=a.GetSettings()
   if err!=nil{return liverecording.Options{},err}
   cookie,err:=a.activeAuthenticationCookiesFile()
   if err!=nil{return liverecording.Options{},err}
   return liverecording.Options{Directory:settings.DownloadDir,CookiesFile:cookie},nil
  },
 )
 if err!=nil{return err}
 recorder.Start(a.appContext())
 a.liveRecorder=recorder
 return nil
}
func (a *App) liveService() (*liverecording.Service,error){
 if err:=a.startLiveRecordingService();err!=nil{return nil,fmt.Errorf("라이브 녹화 서비스를 시작할 수 없습니다: %w",err)}
 a.liveMu.Lock();defer a.liveMu.Unlock()
 return a.liveRecorder,nil
}
func(a *App) GetLiveRecordingState()(liverecording.State,error){
 s,err:=a.liveService();if err!=nil{return liverecording.State{},err}
 return s.Snapshot(),nil
}
func(a *App) ConfigureLiveRecording(pollSeconds,maxConcurrent int)(liverecording.State,error){
 s,err:=a.liveService();if err!=nil{return liverecording.State{},err}
 return s.Configure(pollSeconds,maxConcurrent)
}
func(a *App) SetLiveAutoRecording(channelID,channelName string,enabled bool)(liverecording.State,error){
 s,err:=a.liveService();if err!=nil{return liverecording.State{},err}
 return s.Watch(channelID,channelName,enabled)
}
func(a *App) RemoveLiveAutoRecording(channelID string)(liverecording.State,error){
 s,err:=a.liveService();if err!=nil{return liverecording.State{},err}
 return s.Unwatch(channelID)
}
func(a *App) StartLiveRecording(channelID string)(liverecording.Session,error){
 s,err:=a.liveService();if err!=nil{return liverecording.Session{},err}
 return s.RecordNow(channelID)
}
func(a *App) DeleteLiveRecordingHistory(sessionID string)(liverecording.State,error){
 s,err:=a.liveService();if err!=nil{return liverecording.State{},err}
 return s.DeleteHistory(sessionID)
}
func(a *App) ClearLiveRecordingHistory()(liverecording.State,error){
 s,err:=a.liveService();if err!=nil{return liverecording.State{},err}
 return s.ClearHistory()
}
func(a *App) StopLiveRecording(sessionID string)(bool,error){
 s,err:=a.liveService();if err!=nil{return false,err}
 return s.StopRecording(sessionID),nil
}
func(a *App) ExtractLiveClip(sessionID,title string,startSeconds,endSeconds float64,exact bool)(liverecording.Clip,error){
 s,err:=a.liveService();if err!=nil{return liverecording.Clip{},err}
 return s.ExtractClip(sessionID,title,startSeconds,endSeconds,exact)
}
