package server

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"

    "github.com/google/uuid"
)

func (s *Server) registerParityRoutes() {
    s.mux.HandleFunc("GET /api/overview-stats", s.handleOverviewStats)
    s.mux.HandleFunc("GET /api/providers", s.handleProviders)
    s.mux.HandleFunc("GET /api/providers/presets", s.handleProviderPresets)
    s.mux.HandleFunc("GET /api/providers/presets/config", s.handleProviderPresetsConfig)
    s.mux.HandleFunc("POST /api/providers/discover", s.handleProviderDiscover)
    s.mux.HandleFunc("POST /api/connections/test", s.handleConnectionTest)
    s.mux.HandleFunc("GET /api/models/sync", s.handleModelsSync)
    s.mux.HandleFunc("POST /api/models/sync", s.handleModelsSync)
    s.mux.HandleFunc("GET /api/models/manual", s.handleModelsManual)
    s.mux.HandleFunc("POST /api/models/manual", s.handleModelsManual)
    s.mux.HandleFunc("DELETE /api/models/manual", s.handleModelsManual)
    s.mux.HandleFunc("GET /api/cache", s.handleCache)
    s.mux.HandleFunc("POST /api/cache", s.handleCacheAction)
    s.mux.HandleFunc("GET /api/costs", s.handleCosts)
    s.mux.HandleFunc("GET /api/quota", s.handleQuota)
    s.mux.HandleFunc("GET /api/audit", s.handleAudit)
    s.mux.HandleFunc("GET /api/features", s.handleFeatures)
    s.mux.HandleFunc("GET /api/features/stats", s.handleFeatureStats)
    s.mux.HandleFunc("GET /api/analytics/realtime", s.handleAnalyticsRealtime)
    s.mux.HandleFunc("GET /api/analytics/combos", s.handleAnalyticsCombos)
    s.mux.HandleFunc("GET /api/analytics/stream", s.handleAnalyticsStream)
    s.mux.HandleFunc("POST /api/chat-test", s.handleChatTest)
    s.mux.HandleFunc("POST /api/prompt-routing", s.handlePromptRouting)
    s.mux.HandleFunc("POST /api/prompt-optimizer", s.handlePromptOptimizer)
    s.mux.HandleFunc("GET /api/export", s.handleExport)
    s.mux.HandleFunc("POST /api/sync", s.handleSync)
    s.mux.HandleFunc("GET /api/marketplace", s.handleMarketplace)
    s.mux.HandleFunc("GET /api/oauth", s.handleOAuth)
    s.mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
    s.mux.HandleFunc("GET /api/auth/check", s.handleAuthCheck)
    s.mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
    s.mux.HandleFunc("GET /api/teams/{id}", s.handleTeamByID)
    s.mux.HandleFunc("PUT /api/teams/{id}", s.handleTeamByID)
    s.mux.HandleFunc("DELETE /api/teams/{id}", s.handleTeamByID)
    s.mux.HandleFunc("GET /api/teams/{id}/members", s.handleTeamMembers)
    s.mux.HandleFunc("POST /api/teams/{id}/members", s.handleTeamMembers)
    s.mux.HandleFunc("GET /api/users/{id}", s.handleUserByID)
    s.mux.HandleFunc("PUT /api/users/{id}", s.handleUserByID)
    s.mux.HandleFunc("DELETE /api/users/{id}", s.handleUserByID)
    s.mux.HandleFunc("POST /api/v1/images/generations", s.proxy.HandleImages)
    s.mux.HandleFunc("POST /api/v1/audio/speech", s.proxy.HandleAudioSpeech)
    s.mux.HandleFunc("POST /api/v1/audio/transcriptions", s.proxy.HandleAudioTranscriptions)
    s.mux.HandleFunc("POST /api/web-search", s.handleWebSearch)
}

func (s *Server) handleOverviewStats(w http.ResponseWriter, r *http.Request){ s.handleStats(w,r) }
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request){ s.handleGetConnections(w,r) }

func providerPresets() []map[string]any { return []map[string]any{
    {"id":"openai","name":"OpenAI","description":"GPT models from OpenAI","category":"cloud","baseUrl":"https://api.openai.com","format":"openai","chatPath":"/v1/chat/completions","modelsPath":"/v1/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"anthropic","name":"Anthropic","description":"Claude models","category":"cloud","baseUrl":"https://api.anthropic.com","format":"anthropic","chatPath":"/v1/messages","modelsPath":"/v1/models","authHeader":"x-api-key","authPrefix":""},
    {"id":"openrouter","name":"OpenRouter","description":"Many models through one endpoint","category":"aggregator","baseUrl":"https://openrouter.ai/api","format":"openai","chatPath":"/v1/chat/completions","modelsPath":"/v1/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"sumopod","name":"Sumopod","description":"Sans Sumopod AI gateway","category":"aggregator","baseUrl":"https://ai.sumopod.com","format":"openai","chatPath":"/v1/chat/completions","modelsPath":"/v1/models","authHeader":"Authorization","authPrefix":"Bearer "},
    {"id":"ollama","name":"Ollama","description":"Local Ollama server","category":"local","baseUrl":"http://localhost:11434","format":"openai","chatPath":"/v1/chat/completions","modelsPath":"/v1/models","authHeader":"Authorization","authPrefix":"Bearer ","noAuth":true},
    {"id":"custom","name":"Custom OpenAI-Compatible","description":"Any OpenAI-compatible endpoint","category":"custom","baseUrl":"","format":"openai","chatPath":"/v1/chat/completions","modelsPath":"/v1/models","authHeader":"Authorization","authPrefix":"Bearer "},
} }
func providerCategories() []map[string]any { return []map[string]any{{"id":"cloud","name":"Cloud Providers"},{"id":"aggregator","name":"Aggregators"},{"id":"local","name":"Local"},{"id":"custom","name":"Custom"}} }
func (s *Server) handleProviderPresets(w http.ResponseWriter, r *http.Request){ writeJSON(w, map[string]any{"data":providerPresets(),"categories":providerCategories()}) }
func (s *Server) handleProviderPresetsConfig(w http.ResponseWriter, r *http.Request){ id:=r.URL.Query().Get("id"); for _,p:=range providerPresets(){ if p["id"]==id { writeData(w,p); return } }; writeJSON(w, map[string]any{"data":map[string]any{},"presets":providerPresets(),"formats":[]string{"openai","anthropic","gemini","ollama","custom"}}) }

func (s *Server) handleProviderDiscover(w http.ResponseWriter, r *http.Request){
    var in map[string]any; json.NewDecoder(r.Body).Decode(&in)
    base,_:=in["base_url"].(string); key,_:=in["api_key"].(string); path,_:=in["models_path"].(string); if path==""{path="/v1/models"}
    models, err := fetchModels(base, path, key, "Authorization", "Bearer ")
    if err != nil { writeJSON(w,map[string]any{"success":false,"error":err.Error(),"models":[]any{}}); return }
    writeJSON(w,map[string]any{"success":true,"models":models,"count":len(models)})
}

func fetchModels(base, path, key, h, prefix string)([]any,error){
    if base=="" { return nil, fmt.Errorf("base_url required") }
    req,_:=http.NewRequest("GET", strings.TrimRight(base,"/")+path, nil)
    if key!=""{ if h==""{h="Authorization"}; req.Header.Set(h,prefix+key) }
    c:=&http.Client{Timeout:20*time.Second}; resp,err:=c.Do(req); if err!=nil{return nil,err}; defer resp.Body.Close()
    b,_:=io.ReadAll(resp.Body); if resp.StatusCode>=400 { return nil, fmt.Errorf("upstream status %d: %s",resp.StatusCode,string(b)) }
    var data map[string]any; json.Unmarshal(b,&data)
    if arr,ok:=data["data"].([]any); ok { return arr,nil }
    if arr,ok:=data["models"].([]any); ok { return arr,nil }
    return []any{},nil
}

func (s *Server) handleConnectionTest(w http.ResponseWriter, r *http.Request){
    var in map[string]any; json.NewDecoder(r.Body).Decode(&in); base,_:=in["base_url"].(string); if base==""{base,_=in["baseUrl"].(string)}; key,_:=in["api_key"].(string); if key==""{key,_=in["apiKey"].(string)}; path,_:=in["models_path"].(string); if path==""{path,_=in["modelsPath"].(string)}; if path==""{path="/v1/models"}
    start:=time.Now(); models,err:=fetchModels(base,path,key,"Authorization","Bearer "); if err!=nil{writeJSON(w,map[string]any{"success":false,"error":err.Error(),"latency_ms":time.Since(start).Milliseconds()});return}
    writeJSON(w,map[string]any{"success":true,"message":fmt.Sprintf("Connected successfully · %d models found · %dms", len(models), time.Since(start).Milliseconds()),"latency_ms":time.Since(start).Milliseconds(),"models_count":len(models)})
}

func (s *Server) handleModelsSync(w http.ResponseWriter, r *http.Request){
    if r.Method=="GET" { connID:=r.URL.Query().Get("connection_id"); rows,_:=s.db.Conn().Query("SELECT id, model_id, model_name, owned_by, is_active, discovered_at FROM discovered_models WHERE connection_id=? ORDER BY model_id", connID); out:=[]map[string]any{}; if rows!=nil{defer rows.Close(); for rows.Next(){var id,mid,name,owner,dt string; var active int; rows.Scan(&id,&mid,&name,&owner,&active,&dt); out=append(out,map[string]any{"id":id,"model_id":mid,"model_name":name,"owned_by":owner,"is_active":active,"discovered_at":dt})}}; writeData(w,out); return }
    var in map[string]any; json.NewDecoder(r.Body).Decode(&in); onlyID,_:=in["connection_id"].(string)
    q:="SELECT id, base_url, api_key, models_path, auth_header, auth_prefix FROM connections WHERE is_active=1"; args:=[]any{}; if onlyID!=""{q+=" AND id=?"; args=append(args,onlyID)}
    rows,_:=s.db.Conn().Query(q,args...); synced:=0; if rows!=nil{defer rows.Close(); for rows.Next(){var id,base,key,path,h,p string; rows.Scan(&id,&base,&key,&path,&h,&p); if path==""{path="/v1/models"}; models,err:=fetchModels(base,path,key,h,p); if err==nil{ s.db.Conn().Exec("DELETE FROM discovered_models WHERE connection_id=? AND owned_by!='manual'",id); for _,m:=range models{ mm,_:=m.(map[string]any); mid,_:=mm["id"].(string); if mid==""{continue}; s.db.Conn().Exec("INSERT OR REPLACE INTO discovered_models(id,connection_id,model_id,model_name,owned_by,is_active) VALUES(?,?,?,?,?,1)", uuid.New().String(),id,mid,mid,fmt.Sprint(mm["owned_by"])); synced++ }; s.db.Conn().Exec("UPDATE connections SET models_count=(SELECT COUNT(*) FROM discovered_models WHERE connection_id=? AND is_active=1), last_sync=datetime('now') WHERE id=?", id, id) } } }
    writeJSON(w,map[string]any{"success":true,"data":map[string]any{"synced":synced},"synced":synced})
}
func (s *Server) handleModelsManual(w http.ResponseWriter, r *http.Request){
    connID:=r.URL.Query().Get("connectionId"); if connID==""{connID=r.URL.Query().Get("connection_id")}
    if r.Method=="GET" { rows,_:=s.db.Conn().Query("SELECT id, model_id, model_name, owned_by, is_active, discovered_at FROM discovered_models WHERE connection_id=? ORDER BY model_id", connID); out:=[]map[string]any{}; if rows!=nil{defer rows.Close(); for rows.Next(){var id,mid,name,owner,dt string; var active int; rows.Scan(&id,&mid,&name,&owner,&active,&dt); out=append(out,map[string]any{"id":id,"model_id":mid,"model_name":name,"owned_by":owner,"is_active":active,"discovered_at":dt})}}; writeJSON(w,map[string]any{"models":out,"data":out}); return }
    if r.Method=="DELETE" { mid:=r.URL.Query().Get("modelId"); s.db.Conn().Exec("DELETE FROM discovered_models WHERE connection_id=? AND model_id=?", connID, mid); writeJSON(w,map[string]any{"success":true}); return }
    var in map[string]any; json.NewDecoder(r.Body).Decode(&in); if connID==""{connID,_=in["connectionId"].(string)}; if connID==""{connID,_=in["connection_id"].(string)}
    if in["action"]=="toggle" { mid,_:=in["modelId"].(string); active:=1; if v,ok:=in["active"].(bool);ok&&!v{active=0}; s.db.Conn().Exec("UPDATE discovered_models SET is_active=? WHERE connection_id=? AND model_id=?",active,connID,mid); writeJSON(w,map[string]any{"success":true}); return }
    models:=stringSlice(in["models"]); if len(models)==0{ if m,_:=in["model_id"].(string);m!=""{models=[]string{m}} }
    for _,mid:= range models { s.db.Conn().Exec("INSERT OR REPLACE INTO discovered_models(id,connection_id,model_id,model_name,owned_by,is_active) VALUES(?,?,?,?,?,1)",uuid.New().String(),connID,mid,mid,"manual") }
    s.db.Conn().Exec("UPDATE connections SET models_count=(SELECT COUNT(*) FROM discovered_models WHERE connection_id=? AND is_active=1) WHERE id=?",connID,connID)
    writeJSON(w,map[string]any{"success":true,"data":map[string]any{"count":len(models)}})
}
func (s *Server) handleCache(w http.ResponseWriter,r *http.Request){ var emb,sem int; s.db.Conn().QueryRow("SELECT COUNT(*) FROM embedding_cache").Scan(&emb); s.db.Conn().QueryRow("SELECT COUNT(*) FROM semantic_cache").Scan(&sem); writeJSON(w,map[string]any{"embedding_cache":emb,"semantic_cache":sem,"total":emb+sem}) }
func (s *Server) handleCacheAction(w http.ResponseWriter,r *http.Request){ s.db.Conn().Exec("DELETE FROM embedding_cache"); s.db.Conn().Exec("DELETE FROM semantic_cache"); writeJSON(w,map[string]any{"success":true,"status":"cleared"}) }
func (s *Server) handleCosts(w http.ResponseWriter,r *http.Request){ writeData(w,map[string]any{"today":0,"month":0,"currency":"USD","by_model":[]any{}}) }
func (s *Server) handleQuota(w http.ResponseWriter,r *http.Request){ writeData(w,[]any{map[string]any{"limits":s.getJSONSetting("quota_limits",map[string]any{}),"usage":map[string]any{"requests_today":0,"tokens_today":0}}}) }
func (s *Server) handleAudit(w http.ResponseWriter,r *http.Request){
	rows,_:=s.db.Conn().Query("SELECT id, action, actor, resource, details, created_at FROM audit_events ORDER BY created_at DESC LIMIT 100")
	events:=[]map[string]any{}
	if rows!=nil{defer rows.Close(); for rows.Next(){var id,action,actor,resource,details,created string; rows.Scan(&id,&action,&actor,&resource,&details,&created); events=append(events,map[string]any{"id":id,"action":action,"actor":actor,"resource":resource,"details":details,"created_at":created})}}
	writeData(w,map[string]any{"events":events,"total":len(events)})
}
func (s *Server) handleFeatures(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"features":map[string]bool{"proxy":true,"streaming":true,"dashboard":true,"fallback":true,"cache":true,"plugins":true,"teams":true}}) }
func (s *Server) handleFeatureStats(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"enabled":7,"total":7}) }
func (s *Server) handleAnalyticsRealtime(w http.ResponseWriter,r *http.Request){ s.handleAnalytics(w,r) }
func (s *Server) handleAnalyticsCombos(w http.ResponseWriter,r *http.Request){ writeData(w,map[string]any{"combos":s.getJSONSetting("combos",[]any{}),"stats":[]any{}}) }
func (s *Server) handleAnalyticsStream(w http.ResponseWriter,r *http.Request){ w.Header().Set("Content-Type","text/event-stream"); fmt.Fprintf(w,"data: {\"status\":\"connected\"}\n\n") }
func (s *Server) handleChatTest(w http.ResponseWriter,r *http.Request){ s.proxy.HandleChatCompletions(w,r) }
func (s *Server) handlePromptRouting(w http.ResponseWriter,r *http.Request){ var in map[string]any; json.NewDecoder(r.Body).Decode(&in); writeJSON(w,map[string]any{"recommended_model":"auto","reason":"Go heuristic routing placeholder","input":in}) }
func (s *Server) handlePromptOptimizer(w http.ResponseWriter,r *http.Request){ var in map[string]string; json.NewDecoder(r.Body).Decode(&in); writeJSON(w,map[string]any{"optimized_prompt":strings.TrimSpace(in["prompt"]),"changes":[]string{"trimmed whitespace"}}) }
func (s *Server) handleExport(w http.ResponseWriter,r *http.Request){ w.Header().Set("Content-Type","application/json"); writeJSON(w,map[string]any{"exported_at":time.Now(),"settings":s.getJSONSetting("settings",map[string]any{})}) }
func (s *Server) handleSync(w http.ResponseWriter,r *http.Request){ s.handleModelsSync(w,r) }
func (s *Server) handleMarketplace(w http.ResponseWriter,r *http.Request){ s.handlePluginStore(w,r) }
func (s *Server) handleOAuth(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"status":"not_configured","providers":[]any{}}) }
func (s *Server) handleAuthLogin(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"success":true,"token":"dashboard-session"}) }
func (s *Server) handleAuthCheck(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"authenticated":true}) }
func (s *Server) handleAuthLogout(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"success":true}) }
func (s *Server) handleTeamByID(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"success":true,"id":r.PathValue("id")}) }
func (s *Server) handleTeamMembers(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"team_id":r.PathValue("id"),"members":[]any{}}) }
func (s *Server) handleUserByID(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"success":true,"id":r.PathValue("id")}) }
func (s *Server) handleWebSearch(w http.ResponseWriter,r *http.Request){ writeJSON(w,map[string]any{"results":[]any{},"note":"web search provider not configured"}) }

func (p *ProxyHandler) proxyPath(w http.ResponseWriter, r *http.Request, upstreamPath string){
    body,_:=io.ReadAll(r.Body); var reqBody io.Reader=bytes.NewReader(body)
    conn,err:=p.getFirstConnection(); if err!=nil{http.Error(w,`{"error":"no active connections"}`,404);return}
    upReq,err:=http.NewRequestWithContext(r.Context(),"POST",strings.TrimRight(conn.BaseURL,"/")+upstreamPath,reqBody); if err!=nil{http.Error(w,err.Error(),500);return}
    upReq.Header.Set("Content-Type", r.Header.Get("Content-Type")); if upReq.Header.Get("Content-Type")==""{upReq.Header.Set("Content-Type","application/json")}
    if conn.APIKey!=""{h:=conn.AuthHeader; if h==""{h="Authorization"}; pfx:=conn.AuthPrefix; if pfx==""{pfx="Bearer "}; upReq.Header.Set(h,pfx+conn.APIKey)}
    resp,err:=p.client.Do(upReq); if err!=nil{http.Error(w,fmt.Sprintf(`{"error":"upstream error: %s"}`,err.Error()),502);return}; defer resp.Body.Close()
    for k,v:=range resp.Header{for _,vv:=range v{w.Header().Add(k,vv)}}; w.WriteHeader(resp.StatusCode); io.Copy(w,resp.Body)
}
func (p *ProxyHandler) HandleImages(w http.ResponseWriter,r *http.Request){ p.proxyPath(w,r,"/v1/images/generations") }
func (p *ProxyHandler) HandleAudioSpeech(w http.ResponseWriter,r *http.Request){ p.proxyPath(w,r,"/v1/audio/speech") }
func (p *ProxyHandler) HandleAudioTranscriptions(w http.ResponseWriter,r *http.Request){ p.proxyPath(w,r,"/v1/audio/transcriptions") }
