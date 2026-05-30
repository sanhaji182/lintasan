package server

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) getJSONSetting(key string, fallback any) any {
	v, _ := s.db.GetSetting(key)
	if v == "" { return fallback }
	var out any
	if err := json.Unmarshal([]byte(v), &out); err != nil { return fallback }
	return out
}
func (s *Server) setJSONSetting(key string, v any) {
	b, _ := json.Marshal(v); s.db.SetSetting(key, string(b))
}
func writeJSON(w http.ResponseWriter, v any) { w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode(v) }
func writeData(w http.ResponseWriter, v any) { writeJSON(w, map[string]any{"data": v}) }

func (s *Server) validDashboardAPIKey(key string) bool {
	arr, ok := s.getJSONSetting("api_keys", []any{}).([]any)
	if !ok { return false }
	for _, item := range arr {
		m, _ := item.(map[string]any)
		if m == nil { continue }
		if m["key"] == key && m["disabled"] != true { return true }
	}
	return false
}

func (s *Server) audit(action, actor, resource string, details any) {
	b, _ := json.Marshal(details)
	s.db.Conn().Exec("INSERT INTO audit_events(id, action, actor, resource, details, created_at) VALUES(?,?,?,?,?,datetime('now'))", uuid.New().String(), action, actor, resource, string(b))
}

func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	var total, cached, input, output int; var avg float64
	s.db.Conn().QueryRow("SELECT COUNT(*), COALESCE(SUM(cached),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(AVG(latency_ms),0) FROM request_logs").Scan(&total,&cached,&input,&output,&avg)
	cacheRate:=0.0; if total>0 { cacheRate=float64(cached)/float64(total)*100 }
	daily := []map[string]any{}
	rows,_:=s.db.Conn().Query("SELECT date(created_at), COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0) FROM request_logs GROUP BY date(created_at) ORDER BY date(created_at) DESC LIMIT 30")
	if rows!=nil { defer rows.Close(); for rows.Next(){ var d string; var c,i,o int; rows.Scan(&d,&c,&i,&o); daily=append(daily,map[string]any{"date":d,"requests":c,"input_tokens":i,"output_tokens":o}) } }
	writeJSON(w,map[string]any{"tokensSavedToday":cached*1000,"cacheHitRate":fmt.Sprintf("%.1f",cacheRate),"totalTokensUsed":input+output,"costSaved":float64(cached)*0.002,"avgLatency":avg,"totalRequests":total,"daily":daily,"breakdown":map[string]any{"cached":cached,"direct":total-cached}})
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	providers:=[]map[string]any{}; models:=[]map[string]any{}; daily:=[]map[string]any{}
	rows,_:=s.db.Conn().Query("SELECT provider, COUNT(*), COALESCE(SUM(input_tokens+output_tokens),0) FROM request_logs GROUP BY provider ORDER BY 3 DESC")
	if rows!=nil { defer rows.Close(); for rows.Next(){ var p string; var req,t int; rows.Scan(&p,&req,&t); providers=append(providers,map[string]any{"provider":p,"requests":req,"tokens":t}) } }
	rows,_=s.db.Conn().Query("SELECT model, COUNT(*), COALESCE(SUM(input_tokens+output_tokens),0) FROM request_logs GROUP BY model ORDER BY 3 DESC LIMIT 20")
	if rows!=nil { defer rows.Close(); for rows.Next(){ var m string; var req,t int; rows.Scan(&m,&req,&t); models=append(models,map[string]any{"model":m,"requests":req,"tokens":t}) } }
	rows,_=s.db.Conn().Query("SELECT date(created_at), COUNT(*), COALESCE(SUM(input_tokens+output_tokens),0) FROM request_logs GROUP BY date(created_at) ORDER BY date(created_at) DESC LIMIT 30")
	if rows!=nil { defer rows.Close(); for rows.Next(){ var d string; var req,t int; rows.Scan(&d,&req,&t); daily=append(daily,map[string]any{"date":d,"requests":req,"tokens":t}) } }
	writeJSON(w,map[string]any{"providers":providers,"models":models,"daily":daily})
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	dir:=filepath.Join(s.cfg.DataDir,"backups"); os.MkdirAll(dir,0755)
	files:=[]map[string]any{}
	entries,_:=os.ReadDir(dir)
	for _,e:=range entries{ if info,err:=e.Info();err==nil{ files=append(files,map[string]any{"filename":e.Name(),"size":info.Size(),"created_at":info.ModTime().Format(time.RFC3339)}) } }
	writeData(w,map[string]any{"backups":files})
}
func (s *Server) handleBackupAction(w http.ResponseWriter, r *http.Request) {
	var in map[string]any; json.NewDecoder(r.Body).Decode(&in); action,_:=in["action"].(string)
	dir:=filepath.Join(s.cfg.DataDir,"backups"); os.MkdirAll(dir,0755)
	switch action{
	case "create": name:=fmt.Sprintf("lintasan-%s.db",time.Now().Format("20060102-150405")); data,_:=os.ReadFile(s.cfg.DBPath); os.WriteFile(filepath.Join(dir,name),data,0644); writeJSON(w,map[string]any{"status":"created","filename":name})
	case "export": typ,_:=in["type"].(string); if typ=="analytics"{ var b bytes.Buffer; cw:=csv.NewWriter(&b); cw.Write([]string{"date","requests","tokens"}); cw.Flush(); w.Header().Set("Content-Disposition","attachment; filename=analytics.csv"); w.Write(b.Bytes()); return }; writeJSON(w,map[string]any{"settings":s.getJSONSetting("settings",map[string]any{}),"connections":"masked","exported_at":time.Now()})
	case "delete": name,_:=in["filename"].(string); os.Remove(filepath.Join(dir,filepath.Base(name))); writeJSON(w,map[string]any{"status":"deleted"})
	case "restore": writeJSON(w,map[string]any{"status":"restore_not_implemented_yet"})
	default: writeJSON(w,map[string]any{"error":"unknown action"})
	}
}

func (s *Server) handleFallback(w http.ResponseWriter, r *http.Request){ writeData(w, s.getJSONSetting("fallback_chains", map[string]any{"model_chains":[]any{},"connection_chains":[]any{},"stats":map[string]any{"total_used":0,"success_rate":100}})) }
func (s *Server) handleFallbackAction(w http.ResponseWriter, r *http.Request){ var in map[string]any; json.NewDecoder(r.Body).Decode(&in); data:=s.getJSONSetting("fallback_chains", map[string]any{"model_chains":[]any{},"connection_chains":[]any{}}).(map[string]any); typ,_:=in["type"].(string); key:="model_chains"; if typ=="connection"{key="connection_chains"}; arr:=data[key].([]any); in["id"]=uuid.New().String(); in["usage_count"]=0; data[key]=append(arr,in); s.setJSONSetting("fallback_chains",data); writeJSON(w,map[string]any{"status":"created"}) }
func (s *Server) handleFallbackDelete(w http.ResponseWriter, r *http.Request){ writeJSON(w,map[string]any{"status":"deleted"}) }

func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request){ writeData(w, s.getJSONSetting("api_keys", []any{})) }
func (s *Server) handleKeysAction(w http.ResponseWriter, r *http.Request){ var in map[string]any; json.NewDecoder(r.Body).Decode(&in); action,_:=in["action"].(string); arr:=s.getJSONSetting("api_keys", []any{}).([]any); if action=="create"{ in["id"]=uuid.New().String(); in["key"]="sk-lintasan-"+strings.ReplaceAll(uuid.New().String(),"-",""); in["created_at"]=time.Now().Format(time.RFC3339); arr=append(arr,in); s.setJSONSetting("api_keys",arr); writeJSON(w,in); return }; writeJSON(w,map[string]any{"status":"ok"}) }

func (s *Server) handleLoadBalancer(w http.ResponseWriter,r *http.Request){ v,_:=s.db.GetSetting("lb_strategy"); if v==""{v="priority"}; writeData(w,map[string]any{"strategy":v}) }
func (s *Server) handleLoadBalancerAction(w http.ResponseWriter,r *http.Request){
	var in map[string]string
	json.NewDecoder(r.Body).Decode(&in)
	if in["strategy"] == "" {
		writeJSON(w,map[string]any{"error":"strategy required"})
		return
	}
	s.db.SetSetting("lb_strategy",in["strategy"])
	writeJSON(w,map[string]any{"status":"updated"})
}
func (s *Server) handleAliases(w http.ResponseWriter,r *http.Request){ writeData(w,s.getJSONSetting("aliases",map[string]any{})) }
func (s *Server) handleAliasesDelete(w http.ResponseWriter, r *http.Request){
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	data := s.getJSONSetting("aliases", []any{})

	switch v := data.(type) {
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			m, _ := item.(map[string]any)
			if m == nil { out = append(out, item); continue }
			matchID := fmt.Sprint(m["id"]) == id
			matchName := fmt.Sprint(m["name"]) == id || fmt.Sprint(m["alias"]) == id
			if matchID || matchName { continue }
			out = append(out, item)
		}
		s.setJSONSetting("aliases", out)

	case map[string]any:
		// support alias-as-key and alias-as-name
		if _, ok := v[id]; ok {
			delete(v, id)
		} else {
			for key, item := range v {
				m, _ := item.(map[string]any)
				if m != nil && (fmt.Sprint(m["name"]) == id || fmt.Sprint(m["alias"]) == id) {
					delete(v, key)
				}
			}
		}
		s.setJSONSetting("aliases", v)
	default:
		http.Error(w, `{"error":"aliases unsupported shape"}`, http.StatusBadRequest)
		return
	}
	s.audit("aliases.delete","dashboard",id,map[string]any{})
	writeJSON(w, map[string]any{"id": id, "status": "deleted"})
}
func (s *Server) handleAliasesAction(w http.ResponseWriter,r *http.Request){
	var in map[string]any; json.NewDecoder(r.Body).Decode(&in)
	// Node dashboard posts the whole alias map: {alias: {model: "..."}}
	if _, hasAction := in["action"]; !hasAction {
		if name, _ := in["name"].(string); name != "" {
			existing := s.getJSONSetting("aliases", map[string]any{})
			switch v := existing.(type) {
			case []any:
				found := false
				for i, item := range v {
					m, _ := item.(map[string]any)
					if m != nil && (fmt.Sprint(m["name"]) == name || fmt.Sprint(m["id"]) == name) {
						for k, val := range in { m[k] = val }
						v[i] = m; found = true; break
					}
				}
				if !found { v = append(v, in) }
				s.setJSONSetting("aliases", v); s.audit("aliases.upsert","dashboard",name,in); writeData(w,v); return
			case map[string]any:
				if _, ok := v[name]; !ok { v[name] = map[string]any{"model": fmt.Sprint(in["target"])} }
				s.setJSONSetting("aliases", v); s.audit("aliases.upsert","dashboard",name,in); writeData(w,v); return
			default:
				s.setJSONSetting("aliases", []any{in}); s.audit("aliases.upsert","dashboard",name,in); writeData(w,[]any{in}); return
			}
		}
		s.setJSONSetting("aliases", in); s.audit("aliases.update", "dashboard", "aliases", in); writeData(w,in); return
	}
	aliases, _ := s.getJSONSetting("aliases",map[string]any{}).(map[string]any)
	action,_:=in["action"].(string); alias,_:=in["alias"].(string)
	if action=="delete" && alias!="" { delete(aliases, alias); s.setJSONSetting("aliases",aliases); s.audit("aliases.delete","dashboard",alias,map[string]any{}); writeJSON(w,map[string]any{"success":true}); return }
	model,_:=in["model"].(string); if alias!="" && model!="" { aliases[alias]=map[string]any{"model":model}; s.setJSONSetting("aliases",aliases); s.audit("aliases.create","dashboard",alias,map[string]any{"model":model}); writeData(w,aliases); return }
	writeJSON(w,map[string]any{"error":"alias and model required"})
}

func (s *Server) handlePlugins(w http.ResponseWriter,r *http.Request){ writeData(w,s.getJSONSetting("plugins",[]any{})) }
func (s *Server) handlePluginsAction(w http.ResponseWriter,r *http.Request){ var in map[string]any; json.NewDecoder(r.Body).Decode(&in); arr:=s.getJSONSetting("plugins",[]any{}).([]any); action,_:=in["action"].(string); if action=="create"||action=="install"{ in["id"]=uuid.New().String(); in["enabled"]=true; arr=append(arr,in); s.setJSONSetting("plugins",arr); writeJSON(w,map[string]any{"status":"created"}); return}; writeJSON(w,map[string]any{"status":"ok"}) }
func (s *Server) handlePluginStore(w http.ResponseWriter,r *http.Request){ writeData(w,[]map[string]any{{"name":"Request Logger","category":"observability","author":"Lintasan","version":"1.0.0","description":"Log request metadata","tags":[]string{"logs","debug"}},{"name":"Rate Limiter","category":"security","author":"Lintasan","version":"1.0.0","description":"Basic per-key rate limits","tags":[]string{"rate-limit"}},{"name":"Cost Guard","category":"cost","author":"Lintasan","version":"1.0.0","description":"Block expensive requests","tags":[]string{"cost"}}}) }
func (s *Server) handlePluginStoreAction(w http.ResponseWriter,r *http.Request){ s.handlePluginsAction(w,r) }
func (s *Server) handlePluginGenerate(w http.ResponseWriter,r *http.Request){ var in map[string]any; json.NewDecoder(r.Body).Decode(&in); name,_:=in["name"].(string); if name==""{name="generated-plugin"}; code:=fmt.Sprintf("// %s\nexport default async function plugin(ctx) {\n  return ctx.next();\n}\n",name); writeJSON(w,map[string]any{"name":name,"code":code,"model":"lintasan-go-template"}) }

func (s *Server) handleTeams(w http.ResponseWriter,r *http.Request){ writeData(w,s.getJSONSetting("teams",[]any{})) }
func (s *Server) handleTeamsAction(w http.ResponseWriter,r *http.Request){ var in map[string]any; json.NewDecoder(r.Body).Decode(&in); arr:=s.getJSONSetting("teams",[]any{}).([]any); if in["action"]=="create"||in["name"]!=nil{ in["id"]=uuid.New().String(); in["members"]=[]any{}; arr=append(arr,in); s.setJSONSetting("teams",arr); writeJSON(w,map[string]any{"status":"created"}); return}; writeJSON(w,map[string]any{"status":"ok"}) }
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	// Delegate to auth handler (JWT-based)
	if s.authHandler != nil {
		s.authHandler.HandleListUsers()(w, r)
	} else {
		writeData(w, s.getJSONSetting("users", []any{}))
	}
}

func (s *Server) handleUsersAction(w http.ResponseWriter, r *http.Request) {
	// Delegate to auth handler (JWT-based)
	if s.authHandler != nil {
		s.authHandler.HandleCreateUser()(w, r)
	} else {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		writeJSON(w, map[string]any{"status": "ok"})
	}
}
func (s *Server) handleWebhooks(w http.ResponseWriter,r *http.Request){ writeData(w,s.getJSONSetting("webhooks",map[string]any{"webhooks":[]any{},"history":[]any{}})) }
func (s *Server) handleWebhooksAction(w http.ResponseWriter,r *http.Request){
	var in map[string]any; json.NewDecoder(r.Body).Decode(&in)
	data:=s.getJSONSetting("webhooks",map[string]any{"webhooks":[]any{},"history":[]any{}}).(map[string]any)
	arr:=data["webhooks"].([]any)
	if in["action"]=="test"{ s.deliverWebhooks("test", map[string]any{"message":"Lintasan test webhook","time":time.Now()}); writeJSON(w,map[string]any{"status":"test_sent"}); return }
	// Create when the dashboard posts a webhook payload. The SvelteKit form
	// sends {url, events, secret} with no explicit action/name, so we treat the
	// presence of a url (or action=create / name) as a create intent.
	if in["action"]=="create" || in["name"]!=nil || in["url"]!=nil { in["id"]=uuid.New().String(); in["active"]=true; arr=append(arr,in); data["webhooks"]=arr; s.setJSONSetting("webhooks",data); s.audit("webhook.create","dashboard",fmt.Sprint(in["id"]),in); writeData(w,in); return }
	writeJSON(w,map[string]any{"status":"ok"})
}

func (s *Server) deliverWebhooks(event string, payload map[string]any) {
	data, _ := s.getJSONSetting("webhooks",map[string]any{"webhooks":[]any{}}).(map[string]any)
	arr, _ := data["webhooks"].([]any)
	for _, item := range arr {
		wh, _ := item.(map[string]any); if wh == nil || wh["active"] == false { continue }
		url, _ := wh["url"].(string); if url == "" { continue }
		id := fmt.Sprint(wh["id"])
		body, _ := json.Marshal(map[string]any{"event":event,"payload":payload,"timestamp":time.Now().Format(time.RFC3339)})
		go func(webhookID, target string, b []byte){
			req, _ := http.NewRequest("POST", target, bytes.NewReader(b)); req.Header.Set("Content-Type","application/json")
			resp, err := (&http.Client{Timeout:10*time.Second}).Do(req)
			status:=0; text:=""; if err!=nil{text=err.Error()} else {status=resp.StatusCode; rb,_:=io.ReadAll(io.LimitReader(resp.Body,1024)); text=string(rb); resp.Body.Close()}
			s.db.Conn().Exec("INSERT INTO webhook_deliveries(id, webhook_id, event, status, response, created_at) VALUES(?,?,?,?,?,datetime('now'))", uuid.New().String(), webhookID, event, status, text)
		}(id,url,body)
	}
}

func zipBytes(files map[string][]byte) []byte { var b bytes.Buffer; z:=zip.NewWriter(&b); for n,d:=range files{ f,_:=z.Create(n); f.Write(d)}; z.Close(); return b.Bytes() }

// --- Smart Routing config (dashboard) ---
// GET  /api/smart-routing  → current ML-routing / cost / quota config
// POST /api/smart-routing  → save config, then live-reload proxy in-memory state

func (s *Server) handleSmartRouting(w http.ResponseWriter, r *http.Request) {
	get := func(k, def string) string { v, _ := s.db.GetSetting(k); if v == "" { return def }; return v }
	var quotaLimits any = map[string]any{}
	if raw, _ := s.db.GetSetting("quota_limits"); raw != "" {
		json.Unmarshal([]byte(raw), &quotaLimits)
	}
	writeData(w, map[string]any{
		"ml_router_enabled":        get("ml_router_enabled", "false") == "true",
		"ml_router_cheap_model":    get("ml_router_cheap_model", "gpt-4o-mini"),
		"ml_router_expensive_model": get("ml_router_expensive_model", "gpt-4o"),
		"ml_router_threshold":      get("ml_router_threshold", "0.5"),
		"cost_quality_floor":       get("cost_quality_floor", "0.3"),
		"cost_expensive_anchor":    get("cost_expensive_anchor", "0.02"),
		"quota_limits":             quotaLimits,
	})
}

func (s *Server) handleSmartRoutingAction(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	// String/bool/number scalar settings.
	setStr := func(k string) {
		if v, ok := in[k]; ok {
			switch t := v.(type) {
			case string:
				s.db.SetSetting(k, t)
			case bool:
				if t {
					s.db.SetSetting(k, "true")
				} else {
					s.db.SetSetting(k, "false")
				}
			case float64:
				s.db.SetSetting(k, strconv.FormatFloat(t, 'g', -1, 64))
			}
		}
	}
	setStr("ml_router_enabled")
	setStr("ml_router_cheap_model")
	setStr("ml_router_expensive_model")
	setStr("ml_router_threshold")
	setStr("cost_quality_floor")
	setStr("cost_expensive_anchor")
	// quota_limits is a JSON object stored verbatim.
	if v, ok := in["quota_limits"]; ok {
		if b, err := json.Marshal(v); err == nil {
			s.db.SetSetting("quota_limits", string(b))
		}
	}
	// Live-reload proxy in-memory state so changes apply without a restart.
	if s.proxy != nil {
		s.proxy.ReloadSmartRoutingConfig()
	}
	writeJSON(w, map[string]any{"status": "updated"})
}

