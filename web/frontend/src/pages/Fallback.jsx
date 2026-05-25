import { useEffect, useState } from 'react'

export default function Fallback(){
  const [data,setData]=useState({model_chains:[],connection_chains:[],stats:{}})
  const [form,setForm]=useState({type:'model',name:'',models:''})
  const load=()=>fetch('/api/fallback').then(r=>r.json()).then(d=>setData(d.data||d)).catch(()=>{})
  useEffect(()=>{load()},[])
  async function create(e){e.preventDefault(); await fetch('/api/fallback',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({...form, models: form.models.split(',').map(s=>s.trim()).filter(Boolean)})}); setForm({type:'model',name:'',models:''}); load()}
  const chains=[...(data.model_chains||[]),...(data.connection_chains||[])]
  return <div className="fade-in">
    <p style={{fontSize:13,color:'var(--fg-3)',marginBottom:24}}>Fallback chains are used by Go runtime when upstream providers fail.</p>
    <div style={{display:'grid',gridTemplateColumns:'1fr 1fr 1fr',gap:16,marginBottom:24}}>
      <MiniStat label="MODEL CHAINS" value={(data.model_chains||[]).length}/><MiniStat label="CONNECTION CHAINS" value={(data.connection_chains||[]).length}/><MiniStat label="SUCCESS RATE" value={(data.stats?.success_rate||100)+'%'}/>
    </div>
    <div style={card}>
      <h2 style={title}>Create Fallback Chain</h2>
      <form onSubmit={create} style={{display:'grid',gridTemplateColumns:'160px 1fr 1fr auto',gap:10}}>
        <select style={input} value={form.type} onChange={e=>setForm({...form,type:e.target.value})}><option value="model">Model</option><option value="connection">Connection</option></select>
        <input style={input} placeholder="Chain name" value={form.name} onChange={e=>setForm({...form,name:e.target.value})}/>
        <input style={input} placeholder="model-a, model-b, model-c" value={form.models} onChange={e=>setForm({...form,models:e.target.value})}/>
        <button style={btn}>Create</button>
      </form>
    </div>
    <div style={{...card,marginTop:16}}>{chains.length===0?<p style={{color:'var(--fg-3)',fontSize:13}}>No fallback chains yet.</p>:chains.map(c=><div key={c.id||c.name} style={{padding:14,border:'1px solid var(--border)',borderRadius:8,marginBottom:8,background:'var(--bg-body)'}}><b>{c.name||'Fallback Chain'}</b><div style={{fontSize:12,color:'var(--fg-3)',marginTop:4}}>{(c.models||[]).join(' → ')}</div></div>)}</div>
  </div>
}
function MiniStat({label,value}){return <div style={card}><div style={{fontSize:10,fontWeight:700,color:'var(--fg-3)',letterSpacing:'.08em'}}>{label}</div><div style={{fontSize:24,fontWeight:700,color:'var(--fg-0)',marginTop:6}}>{value}</div></div>}
const card={background:'var(--bg-card)',border:'1px solid var(--border)',borderRadius:'var(--radius)',padding:20,boxShadow:'var(--shadow)'}
const title={fontSize:14,fontWeight:600,color:'var(--fg-0)',margin:'0 0 14px'}
const input={padding:'10px 12px',border:'1px solid var(--border)',borderRadius:8,background:'var(--bg-body)',color:'var(--fg-0)'}
const btn={padding:'10px 14px',background:'var(--primary)',color:'#fff',border:'none',borderRadius:8,fontWeight:600,cursor:'pointer'}
