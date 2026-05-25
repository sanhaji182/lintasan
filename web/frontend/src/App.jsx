import { useEffect, useState } from 'react'
import { BrowserRouter, Routes, Route, NavLink, useNavigate } from 'react-router-dom'
import Overview from './pages/Overview'
import Connections from './pages/Connections'
import Models from './pages/Models'
import Logs from './pages/Logs'
import Settings from './pages/Settings'
import Analytics from './pages/Analytics'
import Usage from './pages/Usage'
import Routing from './pages/Routing'
import Playground from './pages/Playground'
import Keys from './pages/Keys'
import Plugins from './pages/Plugins'
import Teams from './pages/Teams'
import Users from './pages/Users'
import Webhooks from './pages/Webhooks'
import Backup from './pages/Backup'
import Docs from './pages/Docs'
import Fallback from './pages/Fallback'

const NAV_GROUPS = [
  { title: 'MENU', items: [
    ['/dashboard','Overview','▦'], ['/dashboard/connections','Accounts','🔗'], ['/dashboard/routing','Routing','⇄'], ['/dashboard/fallback','Fallback','↻'], ['/dashboard/logs','Logs','▤'], ['/dashboard/usage','Usage','▥'], ['/dashboard/analytics','Analytics','↗'],
  ]},
  { title: 'MANAGE', items: [
    ['/dashboard/keys','API Keys','⚿'], ['/dashboard/teams','Teams','👥'], ['/dashboard/users','Users','👤'], ['/dashboard/webhooks','Webhooks','⌁'], ['/dashboard/backup','Backup','⇩'], ['/dashboard/settings','Settings','⚙'],
  ]},
  { title: 'TOOLS', items: [
    ['/dashboard/plugins','Plugins','▣'], ['/dashboard/playground','Playground','◌'], ['/dashboard/models','Models','◇'], ['/dashboard/docs','Docs','▱'],
  ]},
]

function Login({ onLogin }) {
  const [pass, setPass] = useState('')
  async function login() {
    const r = await fetch('/api/auth/login', { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({ password: pass }) }).catch(()=>null)
    const d = r ? await r.json().catch(()=>({})) : {}
    if (d.success) onLogin(); else alert(d.error || 'Login failed')
  }
  return <div style={{ minHeight:'100vh', display:'flex', alignItems:'center', justifyContent:'center', background:'var(--bg-body)', position:'relative', overflow:'hidden' }}>
    <div style={{ position:'absolute', top:'-20%', right:'-10%', width:500, height:500, borderRadius:'50%', background:'radial-gradient(circle, var(--primary-glow) 0%, transparent 70%)', filter:'blur(60px)' }} />
    <div className="fade-in-scale login-card" style={{ width:420, background:'var(--bg-card)', borderRadius:'var(--radius-lg)', padding:'48px 40px', boxShadow:'var(--shadow-lg)', border:'1px solid var(--border)', position:'relative', backdropFilter:'blur(20px)' }}>
      <div style={{ textAlign:'center', marginBottom:36 }}>
        <div style={{ width:56, height:56, borderRadius:14, background:'linear-gradient(135deg, var(--primary) 0%, #6366f1 100%)', margin:'0 auto 20px', display:'flex', alignItems:'center', justifyContent:'center', boxShadow:'0 8px 24px var(--primary-glow)' }}><span style={{ color:'#fff', fontSize:24, fontWeight:700 }}>L</span></div>
        <h1 style={{ fontSize:24, fontWeight:700, color:'var(--fg-0)', marginBottom:6 }}>Lintasan</h1>
        <p style={{ fontSize:14, color:'var(--fg-2)' }}>Sign in to access your dashboard</p>
      </div>
      <label style={{ display:'block', fontSize:13, fontWeight:500, color:'var(--fg-1)', marginBottom:8 }}>Password</label>
      <input type="password" value={pass} onChange={e=>setPass(e.target.value)} onKeyDown={e=>e.key==='Enter'&&login()} placeholder="Enter your password" style={{ width:'100%', padding:'14px 16px', background:'var(--bg-body)', border:'1px solid var(--border)', borderRadius:'var(--radius-sm)', color:'var(--fg-0)', fontSize:14, outline:'none', marginBottom:20 }} />
      <button onClick={login} style={{ width:'100%', padding:14, background:'linear-gradient(135deg, var(--primary) 0%, #6366f1 100%)', color:'#fff', border:'none', borderRadius:'var(--radius-sm)', fontSize:14, fontWeight:600, cursor:'pointer', boxShadow:'0 4px 12px var(--primary-glow)' }}>Sign In</button>
    </div>
  </div>
}

function Shell() {
  const [auth, setAuth] = useState(false)
  const [checking, setChecking] = useState(true)
  const [theme, setTheme] = useState(() => localStorage.getItem('sr-theme') || 'light')
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const navigate = useNavigate()
  useEffect(()=>{ document.documentElement.setAttribute('data-theme', theme); localStorage.setItem('sr-theme', theme) },[theme])
  useEffect(()=>{ fetch('/api/auth/check').then(r=>r.json()).then(d=>{ setAuth(!!d.authenticated); setChecking(false) }).catch(()=>setChecking(false)) },[])
  if (checking) return <div style={{ minHeight:'100vh', display:'flex', alignItems:'center', justifyContent:'center', background:'var(--bg-body)' }}><div style={{ width:44, height:44, border:'3px solid var(--border)', borderTopColor:'var(--primary)', borderRadius:'50%', animation:'spin .7s linear infinite' }} /></div>
  if (!auth) return <Login onLogin={()=>setAuth(true)} />
  return <div style={{ display:'flex', minHeight:'100vh' }}>
    <div className={`sidebar-backdrop${sidebarOpen?' visible':''}`} onClick={()=>setSidebarOpen(false)} />
    <aside className={`dashboard-sidebar${sidebarOpen?' open':''}`} style={{ width:260, minHeight:'100vh', background:'var(--bg-sidebar)', display:'flex', flexDirection:'column', position:'fixed', left:0, top:0, bottom:0, overflowY:'auto', zIndex:50, borderRight:'1px solid var(--sidebar-border)', boxShadow:'var(--shadow-sm)' }}>
      <div style={{ padding:'20px 16px', display:'flex', alignItems:'center', gap:12, borderBottom:'1px solid var(--sidebar-border)' }}>
        <div style={{ width:36, height:36, borderRadius:10, background:'linear-gradient(135deg, var(--primary) 0%, #6366f1 100%)', display:'flex', alignItems:'center', justifyContent:'center', boxShadow:'0 4px 12px var(--primary-glow)' }}><span style={{ color:'#fff', fontSize:16, fontWeight:700 }}>L</span></div>
        <div><h2 style={{ fontSize:14, fontWeight:700, color:'var(--sidebar-logo-text)', margin:0 }}>Lintasan</h2><p style={{ fontSize:11, color:'var(--fg-3)', margin:0 }}>LLM Gateway</p></div>
      </div>
      <nav style={{ padding:'14px 10px', flex:1 }}>
        {NAV_GROUPS.map(g => <div key={g.title} style={{ marginBottom:18 }}>
          <div style={{ fontSize:10, fontWeight:700, color:'var(--fg-3)', letterSpacing:'.08em', padding:'0 10px 8px' }}>{g.title}</div>
          {g.items.map(([path,label,icon]) => <NavLink key={path} to={path} end={path==='/dashboard'} onClick={()=>setSidebarOpen(false)} style={({isActive})=>({ display:'flex', alignItems:'center', gap:10, padding:'10px 12px', borderRadius:8, textDecoration:'none', fontSize:13, fontWeight:500, marginBottom:2, color:isActive?'var(--primary)':'var(--fg-sidebar)', background:isActive?'var(--primary-light)':'transparent', border:isActive?'1px solid rgba(60,80,224,.16)':'1px solid transparent' })}><span style={{ width:18, textAlign:'center' }}>{icon}</span>{label}</NavLink>)}
        </div>)}
      </nav>
      <div style={{ padding:12, borderTop:'1px solid var(--sidebar-border)' }}><button onClick={()=>setTheme(theme==='dark'?'light':'dark')} style={{ width:'100%', padding:'10px 12px', border:'1px solid var(--border)', borderRadius:8, background:'var(--bg-card)', color:'var(--fg-1)', cursor:'pointer', fontSize:13 }}>{theme==='dark'?'☀ Light':'🌙 Dark'}</button></div>
    </aside>
    <main style={{ marginLeft:260, flex:1, minHeight:'100vh', background:'var(--bg-body)' }}>
      <header style={{ height:64, background:'var(--bg-card)', borderBottom:'1px solid var(--border)', display:'flex', alignItems:'center', justifyContent:'space-between', padding:'0 28px', position:'sticky', top:0, zIndex:20 }}><button className="mobile-menu-btn" onClick={()=>setSidebarOpen(true)} style={{ display:'none' }}>☰</button><div><h1 style={{ fontSize:18, fontWeight:700, color:'var(--fg-0)', margin:0 }}>Dashboard</h1><p style={{ fontSize:12, color:'var(--fg-3)', margin:0 }}>Go v2 runtime · embedded SPA</p></div><button onClick={()=>navigate('/dashboard/playground')} style={{ padding:'9px 14px', background:'var(--primary)', color:'#fff', border:'none', borderRadius:8, fontSize:13, fontWeight:600 }}>Playground</button></header>
      <div style={{ padding:28 }}><Routes>
        <Route path="/dashboard" element={<Overview />} /><Route path="/dashboard/analytics" element={<Analytics />} /><Route path="/dashboard/usage" element={<Usage />} /><Route path="/dashboard/routing" element={<Routing />} /><Route path="/dashboard/fallback" element={<Fallback />} /><Route path="/dashboard/playground" element={<Playground />} /><Route path="/dashboard/connections" element={<Connections />} /><Route path="/dashboard/models" element={<Models />} /><Route path="/dashboard/keys" element={<Keys />} /><Route path="/dashboard/plugins" element={<Plugins />} /><Route path="/dashboard/teams" element={<Teams />} /><Route path="/dashboard/users" element={<Users />} /><Route path="/dashboard/webhooks" element={<Webhooks />} /><Route path="/dashboard/backup" element={<Backup />} /><Route path="/dashboard/logs" element={<Logs />} /><Route path="/dashboard/docs" element={<Docs />} /><Route path="/dashboard/settings" element={<Settings />} />
      </Routes></div>
    </main>
  </div>
}

export default function App(){ return <BrowserRouter><Shell /></BrowserRouter> }
