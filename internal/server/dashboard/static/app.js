// Lintasan Dashboard — Alpine.js + HTMX components
// Auto-generated, do not edit manually
// All page components centralized for HTMX innerHTML compat
// HTMX swaps via innerHTML — script tags in swapped content are NOT executed
// So all Alpine x-data functions must be defined globally, once, in <head>

function analyticsData() {
  return {
    data: null,
    usageData: null,
    providerPerf: [],
    period: 'daily',
    periodLabel: 'Last 24h',

    async fetchData() {
      try {
        // Fetch overview stats
        let r = await fetch('/api/overview-stats');
        if (r.ok) {
          let d = await r.json();
          this.data = d.data || d;
        }

        // Fetch usage data for cost breakdown
        try {
          let u = await fetch('/api/usage');
          if (u.ok) {
            let ud = await u.json();
            this.usageData = ud.data || ud;
            // Compute per-model tokens
            if (this.usageData && this.usageData.models) {
              this.usageData.models = this.usageData.models.map(m => ({
                ...m,
                input_tokens: m.input_tokens || Math.round((m.tokens || 0) * 0.6),
                output_tokens: m.output_tokens || Math.round((m.tokens || 0) * 0.4),
              }));
            }
          }
        } catch(e) { console.error('Usage fetch failed', e); }

        // Build provider performance from overview + usage
        this.buildProviderPerf();

        // Set period label
        this.periodLabel = this.period === 'daily' ? 'Last 24h' : this.period === 'weekly' ? 'Last 7 Days' : 'Last 30 Days';

        // Render chart after Alpine has processed the DOM update
        await this.$nextTick();
        this.renderChart();
      } catch(e) { console.error('Analytics fetch failed', e); }

      // Auto-refresh
      setTimeout(() => this.fetchData(), 30000);
    },

    buildProviderPerf() {
      const providers = (this.data?.providers || []).map(p => ({
        name: p.name,
        format: p.format || '\u2014',
        healthy: p.healthy,
        avg_latency: p.latency || 0,
        success_rate: 100,
        total_tokens: 0,
        total_requests: 0,
      }));

      // Enrich with usage data
      if (this.usageData?.providers) {
        this.usageData.providers.forEach(up => {
          const name = up.provider || up.name;
          const existing = providers.find(p => p.name === name);
          if (existing) {
            existing.total_tokens = up.tokens || 0;
            existing.total_requests = up.requests || 0;
          } else if (name) {
            providers.push({
              name: name,
              format: '\u2014',
              healthy: true,
              avg_latency: this.data?.avgLatency || 0,
              success_rate: 100,
              total_tokens: up.tokens || 0,
              total_requests: up.requests || 0,
            });
          }
        });
      }
      this.providerPerf = providers;
    },

    estimatedCost(model) {
      // Rough estimate: $0.002/1K tokens (average across models)
      const input = model.input_tokens || 0;
      const output = model.output_tokens || 0;
      const tokens = model.tokens || (input + output);
      return (tokens / 1000) * 0.002;
    },

    costPercent(idx) {
      if (!this.usageData?.models) return 0;
      const models = this.usageData.models.slice(0, 10);
      const totalCost = models.reduce((sum, m) => sum + this.estimatedCost(m), 0);
      if (totalCost === 0) return 0;
      return (this.estimatedCost(models[idx]) / totalCost) * 100;
    },

    renderChart() {
      let canvas = document.getElementById('volume-chart');
      if (!canvas || !this.data?.requestVolume) return;

      let ctx = canvas.getContext('2d');
      let values = this.data.requestVolume;
      let labels = this.period === 'daily'
        ? ['00:00','04:00','08:00','12:00','16:00','20:00','Now']
        : this.period === 'weekly'
        ? ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
        : ['W1', 'W2', 'W3', 'W4'];

      // Use device pixel ratio for crisp rendering
      let dpr = window.devicePixelRatio || 1;
      let rect = canvas.parentElement.getBoundingClientRect();
      let w = rect.width;
      let h = 220;

      canvas.width = w * dpr;
      canvas.height = h * dpr;
      canvas.style.width = w + 'px';
      canvas.style.height = h + 'px';
      ctx.scale(dpr, dpr);

      // Clear
      ctx.clearRect(0, 0, w, h);

      let max = Math.max(...values, 1);
      let padding = { top: 30, bottom: 30, left: 10, right: 10 };
      let chartW = w - padding.left - padding.right;
      let chartH = h - padding.top - padding.bottom;
      let barCount = values.length;
      let barGap = Math.max(6, chartW * 0.02);
      let barW = (chartW - barGap * (barCount + 1)) / barCount;

      // Purple gradient for bars
      let gradient = ctx.createLinearGradient(0, h - padding.bottom, 0, padding.top);
      gradient.addColorStop(0, '#6366f1');
      gradient.addColorStop(1, '#a78bfa');

      // Draw bars
      values.forEach((v, i) => {
        let barH = Math.max(2, (v / max) * chartH);
        let x = padding.left + barGap + i * (barW + barGap);
        let y = h - padding.bottom - barH;

        // Bar with rounded top
        let radius = Math.min(4, barW / 2);
        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.moveTo(x, y + radius);
        ctx.arcTo(x, y, x + radius, y, radius);
        ctx.arcTo(x + barW, y, x + barW, y + radius, radius);
        ctx.lineTo(x + barW, h - padding.bottom);
        ctx.lineTo(x, h - padding.bottom);
        ctx.closePath();
        ctx.fill();

        // Value label above bar
        ctx.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--color-fg-2').trim() || '#888';
        ctx.font = 'bold 11px -apple-system, BlinkMacSystemFont, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'bottom';
        ctx.fillText(v, x + barW / 2, y - 4);

        // Label below bar
        ctx.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--color-fg-3').trim() || '#666';
        ctx.font = '10px -apple-system, BlinkMacSystemFont, sans-serif';
        ctx.textBaseline = 'top';
        ctx.fillText(labels[i] || '#' + (i + 1), x + barW / 2, h - padding.bottom + 6);
      });
    }
  };
}

function backupData() {
    return {
      backups: [],
      loading: true,
      creating: false,
      exporting: null,
      toasts: [],

      init() {
        this.fetchBackups();
      },

      async fetchBackups() {
        this.loading = true;
        try {
          let r = await fetch('/api/backup', { credentials: 'include' });
          if (r.ok) {
            let data = await r.json();
            this.backups = data.backups || data.data || [];
          }
        } catch(e) { console.error('Backup fetch failed', e); }
        this.loading = false;
      },

      async createBackup() {
        this.creating = true;
        this.showToast('info', 'Creating backup...');
        try {
          let r = await fetch('/api/backup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ action: 'create' })
          });
          if (r.ok) {
            this.showToast('success', 'Backup created successfully');
          } else {
            this.showToast('error', 'Failed to create backup');
          }
        } catch(e) {
          console.error('Create backup failed', e);
          this.showToast('error', 'Failed to create backup');
        }
        this.creating = false;
        this.fetchBackups();
      },

      async handleExport(type) {
        this.exporting = type;
        try {
          let r = await fetch('/api/backup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ action: 'export', type })
          });
          if (r.ok) {
            let blob = await r.blob();
            let url = URL.createObjectURL(blob);
            let a = document.createElement('a');
            a.href = url;
            let ext = type === 'analytics' ? 'csv' : 'json';
            a.download = 'lintasan-' + type + '-' + new Date().toISOString().slice(0, 10) + '.' + ext;
            a.click();
            URL.revokeObjectURL(url);
            this.showToast('success', 'Export completed');
          } else {
            this.showToast('error', 'Export failed');
          }
        } catch(e) {
          console.error('Export failed', e);
          this.showToast('error', 'Export failed');
        }
        this.exporting = null;
      },

      async handleRestore(filename) {
        if (!confirm('Restore from backup "' + filename + '"? This will overwrite current configuration.')) return;
        if (!confirm('Are you absolutely sure? This action cannot be undone.')) return;
        try {
          let r = await fetch('/api/backup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ action: 'restore', filename })
          });
          if (r.ok) {
            this.showToast('success', 'Restore initiated');
          } else {
            this.showToast('error', 'Restore failed');
          }
        } catch(e) {
          console.error('Restore failed', e);
          this.showToast('error', 'Restore failed');
        }
        this.fetchBackups();
      },

      async handleDelete(filename) {
        if (!confirm('Delete backup "' + filename + '"?')) return;
        try {
          let r = await fetch('/api/backup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ action: 'delete', filename })
          });
          if (r.ok) {
            this.showToast('success', 'Backup deleted');
          } else {
            this.showToast('error', 'Delete failed');
          }
        } catch(e) {
          console.error('Delete failed', e);
          this.showToast('error', 'Delete failed');
        }
        this.fetchBackups();
      },

      showToast(type, message) {
        this.toasts.push({ type, message });
      },

      formatSize(bytes) {
        if (!bytes) return '\u2014';
        if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB';
        if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return bytes + ' B';
      },

      formatDate(d) {
        if (!d) return '\u2014';
        return new Date(d).toLocaleString();
      }
    };
  }

(function(){
  window.val = function(v){ return v != null ? v : 0; };
  window.val2 = function(alt){ return function(v){ return v != null ? v : alt; }; };

  // Shared data getter
  function getConnData(){
    var e = document.querySelector('[x-data^="connectionsPage"]');
    if(!e) e = document.querySelector('*[x-data*="connectionsPage"]');
    if(!e) e = document.querySelector('[x-data]');
    return e ? Alpine.$data(e) : null;
  }

  window.connectionsPage = function(){
    return {
      connections: null,
      stats: { providers: 0, active: 0, totalModels: 0 },
      showModal: false,
      step1: true,
      selectedPreset: null,
      presets: null,
      presetCategories: [],
      form: { name: '', provider: '', api_key: '', base_url: '', format: 'openai', chat_path: '/v1/chat/completions', models_path: '/v1/models', auth_header: 'Authorization', auth_prefix: 'Bearer ' },
      error: '',
      testing: false,
      resultOk: false,
      resultFail: false,
      testMessage: '',
      deleteTarget: null,
      expandedRows: {},
      connModels: {},
      connTestResults: {},
      addShow: {},
      addValue: {},
      addSaving: {},
      toasts: [],

      faviconURL: function(url){
        if (!url) return '/api/favicon?domain=example.com';
        try { return '/api/favicon?domain=' + encodeURIComponent(new URL(url).hostname); }
        catch(e) { return '/api/favicon?domain=example.com'; }
      },

      catPresets: function(catId){
        var p = this.presets || [];
        return p.filter(function(x){ return (x.category || 'other') === catId; });
      },

      dotStyle: function(active){
        return 'width:10px;height:10px;border-radius:50%;background:' + (active ? 'var(--success)' : 'var(--fg-3)') + ';box-shadow:' + (active ? '0 0 6px rgba(16,185,129,0.4)' : 'none');
      },

      fmtBadgeStyle: function(fmt){
        var bg = 'rgba(16,185,129,0.12)', c = 'var(--success)';
        if (fmt === 'anthropic') { bg = 'rgba(139,92,246,0.12)'; c = 'var(--purple)'; }
        else if (fmt === 'commandcode') { bg = 'rgba(59,130,246,0.12)'; c = 'var(--info)'; }
        return 'background:' + bg + ';color:' + c;
      },

      chevronStyle: function(id){
        return this.expandedRows[id] ? 'transform:rotate(90deg)' : 'transform:rotate(0deg)';
      }
    };
  };

  // helper: get connection by data-conn-id
  function getConnById(connId){
    var dd = getConnData(); if(!dd || !dd.connections) return null;
    return dd.connections.find(function(c){ return String(c.id) === String(connId); });
  }
  function getConnId(el){
    // Walk up to find data-conn-id attribute
    while(el && !el.getAttribute('data-conn-id')){
      el = el.parentElement;
    }
    return el ? el.getAttribute('data-conn-id') : null;
  }

  // Expose helpers for debugging
  window.__getConnId = getConnId;
  window.__getConnById = getConnById;

  window.cs = {
    openAdd: function(){
      var dd = getConnData(); if(!dd) return;
      dd.showModal = true; dd.step1 = true; dd.selectedPreset = null;
      dd.error = ''; dd.resultOk = false; dd.resultFail = false; dd.testing = false;
      dd.form = { name: '', provider: '', api_key: '', base_url: '', format: 'openai', chat_path: '/v1/chat/completions', models_path: '/v1/models', auth_header: 'Authorization', auth_prefix: 'Bearer ' };
      cs.loadPresets();
    },
    hideModal: function(){ var dd = getConnData(); if(dd){ dd.showModal = false; dd.error = ''; } },
    loadPresets: async function(){
      var dd = getConnData(); if(!dd) return;
      try {
        var r = await fetch('/api/providers/presets');
        if(r.ok){ var j = await r.json(); dd.presets = Array.isArray(j.data) ? j.data : []; dd.presetCategories = j.categories || []; }
        else { dd.presets = []; dd.presetCategories = []; }
      } catch(e) { dd.presets = []; dd.presetCategories = []; }
    },
    selectPreset: function(btn){
      var dd = getConnData(); if(!dd) return;
      var id = btn.getAttribute('data-id');
      var p = (dd.presets||[]).find(function(x){ return x.id === id; });
      if(!p) return;
      dd.selectedPreset = p; dd.step1 = false;
      dd.form = {
        name: p.name || '', provider: p.id || '',
        api_key: '', base_url: p.base_url || p.baseUrl || '',
        format: p.format || 'openai',
        chat_path: p.chat_path || p.chatPath || '/v1/chat/completions',
        models_path: p.models_path || p.modelsPath || '/v1/models',
        auth_header: p.auth_header || p.authHeader || 'Authorization',
        auth_prefix: p.auth_prefix || p.authPrefix || 'Bearer '
      };
      dd.error = ''; dd.resultOk = false; dd.resultFail = false;
    },
    selectCustom: function(){
      var dd = getConnData(); if(!dd) return;
      dd.selectedPreset = { id: 'custom', name: 'Custom' }; dd.step1 = false;
      dd.form = { name: '', provider: 'custom', api_key: '', base_url: '', format: 'openai', chat_path: '/v1/chat/completions', models_path: '/v1/models', auth_header: 'Authorization', auth_prefix: 'Bearer ' };
    },
    backToPresets: function(){
      var dd = getConnData(); if(!dd) return;
      dd.step1 = true; dd.selectedPreset = null; dd.error = '';
      dd.resultOk = false; dd.resultFail = false;
    },
    testConnection: async function(){
      var dd = getConnData(); if(!dd) return;
      if(dd.testing) return;
      dd.testing = true; dd.resultOk = false; dd.resultFail = false;
      try {
        var r = await fetch('/api/connections/test', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ base_url: dd.form.base_url, api_key: dd.form.api_key, models_path: dd.form.models_path }) });
        var j = await r.json();
        if(j.success){ dd.resultOk = true; dd.testMessage = j.message || 'Connected'; }
        else { dd.resultFail = true; dd.testMessage = j.message || j.error || 'Failed'; }
      } catch(e) { dd.resultFail = true; dd.testMessage = e.message; }
      dd.testing = false;
    },
    saveConnection: async function(){
      var dd = getConnData(); if(!dd) return;
      if(!dd.form.name.trim()){ dd.error = 'Name required'; return; }
      if(!dd.resultOk){ dd.error = 'Test connection first'; return; }
      try {
        var r = await fetch('/api/connections', { method: 'POST', headers: {'Content-Type':'application/json'},
          body: JSON.stringify({ name: dd.form.name, provider: dd.form.provider, base_url: dd.form.base_url, api_key: dd.form.api_key, format: dd.form.format, chat_path: dd.form.chat_path, models_path: dd.form.models_path, auth_header: dd.form.auth_header, auth_prefix: dd.form.auth_prefix, is_active: 1 }) });
        var j = await r.json();
        if(j.error){ dd.error = j.error.message || 'Failed'; return; }
        dd.showModal = false; dd.toasts.push('Provider connected');
        csCSFcsCSFfetchConnections(); csCSFcsCSFfetchStats();
        var connId = (j.data && j.data.id) || j.id;
        if(connId){ try{ await fetch('/api/models/sync',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({connection_id:connId})}); csCSFcsCSFfetchConnections(); csCSFcsCSFfetchStats(); }catch(e){} }
      } catch(e) { dd.error = 'Network error'; }
    },
    testSingle: async function(btn){
      var dd = getConnData(); if(!dd) return;
      var connId = getConnId(btn);
      var conn = getConnById(connId); if(!conn) return;
      dd.connTestResults[conn.id] = null;
      try {
        var r = await fetch('/api/connections/test',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({base_url:conn.base_url,api_key:conn.api_key})});
        var j = await r.json();
        dd.connTestResults[conn.id] = { success: j.success || false, msg: (j.message || j.error || 'Done'), ms: j.latency_ms || 0 };
      } catch(e) { dd.connTestResults[conn.id] = { success: false, msg: e.message, ms: 0 }; }
    },
    syncSingle: async function(btn){
      var dd = getConnData(); if(!dd) return;
      var connId = getConnId(btn);
      var conn = getConnById(connId); if(!conn) return;
      try {
        var r = await fetch('/api/models/sync',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({connection_id:conn.id})});
        if(r.ok){ await csCSFcsCSFfetchConnections(); await csCSFcsCSFfetchStats(); dd.toasts.push('Synced ' + conn.name); }
        else dd.toasts.push('Sync failed');
      } catch(e) { dd.toasts.push('Sync error'); }
    },
    toggleStatus: async function(btn){
      var dd = getConnData(); if(!dd) return;
      var connId = getConnId(btn);
      var conn = getConnById(connId); if(!conn) return;
      try {
        var r = await fetch('/api/connections?id=' + encodeURIComponent(conn.id), { method: 'PATCH', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ is_active: conn.is_active ? 0 : 1 }) });
        if(r.ok){ conn.is_active = !conn.is_active; csCSFfetchStats(); dd.toasts.push(conn.is_active ? conn.name + ' enabled' : conn.name + ' disabled'); }
      } catch(e){}
    },
    toggleExpand: function(el){
      var dd = getConnData(); if(!dd) return;
      var connId = getConnId(el);
      var conn = getConnById(connId); if(!conn) return;
      dd.expandedRows[conn.id] = !dd.expandedRows[conn.id];
      if (dd.expandedRows[conn.id] && !dd.connModels[conn.id]) {
        cs.loadModels(conn.id);
      }
    },
    loadModels: async function(connId){
      var dd = getConnData(); if(!dd) return;
      try {
        var r = await fetch('/api/models/manual?connectionId=' + encodeURIComponent(connId));
        if(r.ok) { var j = await r.json(); dd.connModels[connId] = j.models || j.data || []; }
      } catch(e){ dd.connModels[connId] = []; }
    },
    addModelShow: function(ev){
      var dd = getConnData(); if(!dd) return;
      var connId = ev.target.getAttribute('data-conn-id') || getConnId(ev.target);
      dd.addShow[connId] = true; dd.addValue[connId] = '';
      setTimeout(function(){
        var el = document.getElementById('add-model-' + connId);
        if(el) el.focus();
      }, 50);
    },
    addModelSubmit: async function(ev, el){
      var dd = getConnData(); if(!dd) return;
      var connId = el.getAttribute('data-conn-id') || getConnId(el);
      var input = document.getElementById('add-model-' + connId);
      var value = input ? input.value : (dd.addValue[connId] || '');
      if(!value.trim()) return;
      dd.addSaving[connId] = true;
      var models = value.split(',').map(function(m){ return m.trim(); }).filter(Boolean);
      try {
        var r = await fetch('/api/models/manual', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ connectionId: connId, models: models }) });
        if(r.ok){ dd.addShow[connId] = false; dd.addValue[connId] = ''; cs.loadModels(connId); csCSFfetchConnections(); }
      } catch(e){}
      dd.addSaving[connId] = false;
    },
    addModelCancel: function(ev){
      var dd = getConnData(); if(!dd) return;
      var connId = ev.target.getAttribute('data-conn-id') || getConnId(ev.target);
      dd.addShow[connId] = false; dd.addValue[connId] = '';
    },
    toggleModelActive: async function(ev, btn){
      ev.stopPropagation();
      var dd = getConnData(); if(!dd) return;
      var connId = btn.getAttribute('data-conn-id');
      var modelId = btn.getAttribute('data-model-id');
      var curActive = parseInt(btn.getAttribute('data-is-active')) || 0;
      try {
        var r = await fetch('/api/models/manual', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({ action: 'toggle', connectionId: connId, modelId: modelId, active: curActive ? false : true }) });
        if(r.ok){ cs.loadModels(connId); csCSFfetchConnections(); }
      } catch(e){}
    },
    removeModel: async function(ev, btn){
      ev.stopPropagation();
      var dd = getConnData(); if(!dd) return;
      var connId = btn.getAttribute('data-conn-id');
      var modelId = btn.getAttribute('data-model-id');
      try {
        var r = await fetch('/api/models/manual?connectionId=' + encodeURIComponent(connId) + '&modelId=' + encodeURIComponent(modelId), { method: 'DELETE' });
        if(r.ok){ cs.loadModels(connId); csCSFfetchConnections(); }
      } catch(e){}
    },
    confirmDelete: function(btn){
      var dd = getConnData(); if(!dd) return;
      var connId = getConnId(btn);
      var conn = getConnById(connId); if(!conn) return;
      dd.deleteTarget = { id: conn.id, name: conn.name };
    },
    cancelDelete: function(){ var dd = getConnData(); if(dd) dd.deleteTarget = null; },
    doDelete: async function(){
      var dd = getConnData(); if(!dd || !dd.deleteTarget) return;
      try {
        var r = await fetch('/api/connections/' + encodeURIComponent(dd.deleteTarget.id), { method: 'DELETE' });
        if(r.ok){ dd.deleteTarget = null; await csCSFcsCSFfetchConnections(); await csCSFcsCSFfetchStats(); dd.toasts.push('Deleted'); }
        else dd.toasts.push('Delete failed');
      } catch(e){ dd.toasts.push('Delete error'); }
    },
    syncAll: async function(){
      var dd = getConnData(); if(!dd) return;
      try {
        var r = await fetch('/api/models/sync', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({}) });
        if(r.ok){ await csCSFcsCSFfetchConnections(); await csCSFcsCSFfetchStats(); dd.toasts.push('All synced'); }
      } catch(e){}
    }
  };

  window.csCSFfetchStats = async function(){
    var dd = getConnData(); if(!dd){ setTimeout(csCSFfetchStats,100); return; }
    try {
      var r = await fetch('/api/connections');
      if(r.ok){ var data = await r.json(); var conns = Array.isArray(data) ? data : (data.data||[]); dd.stats = { providers: conns.length, active: conns.filter(function(c){return c.is_active;}).length, totalModels: conns.reduce(function(s,c){return s+(c.models_count||0);},0) }; }
    } catch(e){}
  };
  window.csCSFfetchConnections = async function(){
    var dd = getConnData(); if(!dd){ setTimeout(csCSFfetchConnections,100); return; }
    try {
      var r = await fetch('/api/connections');
      if(r.ok){ var data = await r.json(); var conns = Array.isArray(data) ? data : (data.data||[]); dd.connections = conns; csCSFfetchStats(); }
    } catch(e){}
  };
})();

function fallbackPage() {
  return {
    modelChains: [],
    connectionChains: [],
    stats: { total_used: 0, success_rate: 100 },
    loading: true,
    showForm: false,
    formType: 'model',
    form: { name: '', models: '' },
    toasts: [],

    addToast: function(type, message) {
      this.toasts.push({ type: type, message: message });
    },
  };
}

(function() {
  function getData() {
    var el = document.querySelector('[x-data]');
    return el ? Alpine.$data(el) : null;
  }

  window.fetchChains = async function() {
    var d = getData();
    if (!d) { setTimeout(fetchChains, 100); return; }
    try {
      var r = await fetch('/api/fallback', { credentials: 'include' });
      if (r.ok) {
        var json = await r.json();
        var raw = json.data || json;
        // Handle both array and object formats
        var mc = raw.model_chains || [];
        var cc = raw.connection_chains || [];
        d.modelChains = Array.isArray(mc) ? mc : Object.entries(mc).map(function(e) { return { name: e[0], models: Array.isArray(e[1]) ? e[1] : [] }; });
        d.connectionChains = Array.isArray(cc) ? cc : Object.entries(cc).map(function(e) { return { name: e[0], models: Array.isArray(e[1]) ? e[1] : [] }; });
        d.stats = raw.stats || { total_used: 0, success_rate: 100 };
      }
    } catch(e) { console.error('Fallback fetch failed', e); }
    d.loading = false;
  };

  window.fs = {
    showCreateForm: function() {
      var d = getData(); if (!d) return;
      d.formType = 'model';
      d.form = { name: '', models: '' };
      d.showForm = true;
    },

    hideCreateForm: function() {
      var d = getData(); if (!d) return;
      d.showForm = false;
    },

    handleCreate: async function() {
      var d = getData(); if (!d) return;
      var name = (d.form.name || '').trim();
      var modelsRaw = (d.form.models || '').trim();
      if (!name || !modelsRaw) return;

      var models = modelsRaw.split(',').map(function(m) { return m.trim(); }).filter(Boolean);
      try {
        var r = await fetch('/api/fallback', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: JSON.stringify({ type: d.formType, id: name, fallbacks: models })
        });
        if (r.ok) {
          d.showForm = false;
          d.form = { name: '', models: '' };
          d.addToast('success', 'Fallback chain created');
          await fetchChains();
        } else {
          d.addToast('error', 'Failed to create chain');
        }
      } catch(e) {
        console.error('Create chain failed', e);
        d.addToast('error', 'Failed to create chain');
      }
    },

    deleteChain: async function(btn, type) {
      var d = getData(); if (!d) return;
      var row = btn.closest('tr');
      var tbody = row.parentNode;
      var rows = tbody.querySelectorAll(':scope > tr');
      var idx = Array.prototype.indexOf.call(rows, row);
      var chains = type === 'model' ? d.modelChains : d.connectionChains;
      var chain = chains[idx];
      if (!chain) return;
      var chainName = chain.name || chain.id || 'this chain';
      if (!confirm('Remove fallback chain "' + chainName + '"?')) return;

      try {
        var url = '/api/fallback?type=' + encodeURIComponent(type) + '&id=' + encodeURIComponent(chain.id || chain.name || '');
        var r = await fetch(url, { method: 'DELETE', credentials: 'include' });
        if (r.ok) {
          if (type === 'model') {
            d.modelChains = d.modelChains.filter(function(c) { return c !== chain; });
          } else {
            d.connectionChains = d.connectionChains.filter(function(c) { return c !== chain; });
          }
          d.addToast('success', 'Chain deleted');
        } else {
          d.addToast('error', 'Failed to delete chain');
        }
      } catch(e) {
        console.error('Delete chain failed', e);
        d.addToast('error', 'Failed to delete chain');
      }
    },
  };
})();

function keysData() {
  return {
    keys: null,
    showForm: false,
    newKey: { name: '', daily_limit: '', monthly_limit: '' },
    toasts: [],

    get withLimits() {
      return (this.keys || []).filter(k => k.daily_limit || k.monthly_limit).length;
    },
    get unlimited() {
      return (this.keys || []).filter(k => !k.daily_limit && !k.monthly_limit).length;
    },

    async fetchKeys() {
      try {
        let r = await fetch('/api/keys');
        if (r.ok) {
          let data = await r.json();
          this.keys = Array.isArray(data) ? data : (data.data || []);
        }
      } catch(e) { console.error('Keys fetch failed', e); }
    },

    async createKey() {
      if (!this.newKey.name.trim()) return;
      try {
        let body = { name: this.newKey.name };
        if (this.newKey.daily_limit) body.daily_limit = parseInt(this.newKey.daily_limit);
        if (this.newKey.monthly_limit) body.monthly_limit = parseInt(this.newKey.monthly_limit);
        let r = await fetch('/api/keys', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body)
        });
        if (r.ok) {
          let data = await r.json();
          let newKey = Array.isArray(data) ? data[data.length-1] : data;
          if (!Array.isArray(data)) newKey = data;
          if (this.keys) {
            this.keys.unshift(newKey);
          } else {
            this.keys = [newKey];
          }
          this.showForm = false;
          this.newKey = { name: '', daily_limit: '', monthly_limit: '' };
          this.showToast('success', 'API key created successfully');
        }
      } catch(e) { console.error('Create key failed', e); this.showToast('error', 'Failed to create key'); }
    },

    async deleteKey(key, idx) {
      if (!confirm('Delete API key "' + key.name + '"? This cannot be undone.')) return;
      try {
        let r = await fetch('/api/keys?id=' + key.id, { method: 'DELETE' });
        if (r.ok) {
          this.keys.splice(idx, 1);
          this.showToast('success', 'API key deleted');
        }
      } catch(e) { console.error('Delete key failed', e); this.showToast('error', 'Failed to delete key'); }
    },

    async copyKey(key) {
      if (!key.key) return;
      try {
        await navigator.clipboard.writeText(key.key);
        this.showToast('success', 'Key copied to clipboard');
      } catch(e) {
        let inp = document.createElement('input');
        inp.value = key.key;
        document.body.appendChild(inp);
        inp.select();
        document.execCommand('copy');
        document.body.removeChild(inp);
        this.showToast('success', 'Key copied to clipboard');
      }
    },

    showToast(type, message) {
      let toast = { type, message };
      this.toasts.push(toast);
      setTimeout(() => {
        let idx = this.toasts.indexOf(toast);
        if (idx > -1) this.toasts.splice(idx, 1);
      }, 3000);
    }
  };
}

function logsApp() {
  return {
    allLogs: [],
    loading: true,
    filter: '',
    level: 'all',
    page: 1,
    perPage: 25,
    sseConnected: false,
    es: null,

    get filteredLogs() {
      let logs = this.allLogs;
      if (this.level !== 'all') {
        logs = logs.filter(l => l.level === this.level);
      }
      if (this.filter) {
        const q = this.filter.toLowerCase();
        logs = logs.filter(l => {
          return (l.provider || '').toLowerCase().includes(q) ||
                 (l.model || '').toLowerCase().includes(q) ||
                 (String(l.status) || '').toLowerCase().includes(q) ||
                 (l.error || '').toLowerCase().includes(q) ||
                 (l.message || '').toLowerCase().includes(q);
        });
      }
      return logs;
    },

    get totalPages() {
      return Math.max(1, Math.ceil(this.filteredLogs.length / this.perPage));
    },

    get paginatedLogs() {
      const p = Math.min(this.page, this.totalPages);
      return this.filteredLogs.slice((p - 1) * this.perPage, p * this.perPage);
    },

    get successCount() {
      return this.allLogs.filter(l => (l.status || 0) < 400).length;
    },

    get errorCount() {
      return this.allLogs.filter(l => (l.status || 0) >= 400).length;
    },

    get avgLatency() {
      const logs = this.allLogs.filter(l => l.latency_ms > 0);
      if (!logs.length) return 0;
      return Math.round(logs.reduce((s, l) => s + l.latency_ms, 0) / logs.length);
    },

    init() {
      window.logsScope = this;
      this.fetchLogs();
      this.connectSSE();
      setTimeout(() => this.init(), 30000);
    },

    async fetchLogs() {
      try {
        const r = await fetch('/api/logs?limit=100', { credentials: 'include' });
        if (r.ok) {
          const d = await r.json();
          this.allLogs = d.data || d || [];
          this.loading = false;
        }
      } catch(e) {
        console.error('Logs fetch failed', e);
        this.loading = false;
      }
    },

    refresh() {
      this.loading = true;
      this.fetchLogs();
    },

    connectSSE() {
      if (this.es) {
        this.es.close();
      }
      try {
        this.es = new EventSource('/api/logs/stream');
        this.es.addEventListener('message', (e) => {
          try {
            const log = JSON.parse(e.data);
            this.allLogs.unshift(log);
            if (this.allLogs.length > 500) {
              this.allLogs.pop();
            }
          } catch(err) {}
        });
        this.es.addEventListener('error', () => {
          this.sseConnected = false;
          setTimeout(() => this.connectSSE(), 5000);
        });
        this.es.onopen = () => { this.sseConnected = true; };
      } catch(e) {
        console.error('SSE connect failed', e);
      }
    },

    modelName(model) {
      if (!model) return '—';
      const parts = model.split('/');
      return parts[parts.length - 1];
    },

    fmtNum(n) {
      if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
      if (n >= 1000) return Math.round(n / 1000) + 'K';
      return String(n);
    },

    timeAgo(ts) {
      if (!ts) return '—';
      const d = Date.now() - new Date(ts).getTime();
      if (d < 60000) return 'just now';
      if (d < 3600000) return Math.floor(d / 60000) + 'm ago';
      if (d < 86400000) return Math.floor(d / 3600000) + 'h ago';
      return Math.floor(d / 86400000) + 'd ago';
    }
  };
}

// Reset page on navigation
document.addEventListener('alpine:init', () => {});

function metricsData() {
  return {
    analytics: null,
    usage: null,

    async fetchMetrics() {
      try {
        // Fetch analytics stats
        let a = await fetch('/api/analytics');
        if (a.ok) {
          this.analytics = await a.json();
        }
      } catch(e) { console.error('Analytics fetch failed', e); }

      try {
        // Fetch usage data for provider & model breakdown
        let u = await fetch('/api/usage');
        if (u.ok) {
          this.usage = await u.json();
        }
      } catch(e) { console.error('Usage fetch failed', e); }

      // Render chart after DOM update
      await this.$nextTick();
      this.renderChart();

      // Auto-refresh
      setTimeout(() => this.fetchMetrics(), 30000);
    },

    providerPercent(idx) {
      if (!this.usage?.providers) return 0;
      const providers = this.usage.providers;
      const totalTokens = providers.reduce((sum, p) => sum + (p.tokens || 0), 0);
      if (totalTokens === 0) return 0;
      return ((providers[idx]?.tokens || 0) / totalTokens) * 100;
    },

    modelPercent(idx) {
      if (!this.usage?.models) return 0;
      const models = this.usage.models.slice(0, 15);
      const totalTokens = models.reduce((sum, m) => sum + (m.tokens || 0), 0);
      if (totalTokens === 0) return 0;
      return ((models[idx]?.tokens || 0) / totalTokens) * 100;
    },

    renderChart() {
      let canvas = document.getElementById('metrics-volume-chart');
      if (!canvas || !this.analytics?.daily) return;

      let ctx = canvas.getContext('2d');
      let daily = this.analytics.daily || [];
      // Sort by date ascending
      daily = [...daily].sort((a, b) => (a.date || '').localeCompare(b.date || ''));
      let values = daily.map(d => d.requests || 0);
      let labels = daily.map(d => {
        let parts = (d.date || '').split('-');
        return parts.length >= 2 ? parts[1] + '/' + parts[2] : d.date || '';
      });

      // Use device pixel ratio for crisp rendering
      let dpr = window.devicePixelRatio || 1;
      let rect = canvas.parentElement.getBoundingClientRect();
      let w = rect.width;
      let h = 220;

      canvas.width = w * dpr;
      canvas.height = h * dpr;
      canvas.style.width = w + 'px';
      canvas.style.height = h + 'px';
      ctx.scale(dpr, dpr);

      // Clear
      ctx.clearRect(0, 0, w, h);

      let max = Math.max(...values, 1);
      let padding = { top: 30, bottom: 40, left: 10, right: 10 };
      let chartW = w - padding.left - padding.right;
      let chartH = h - padding.top - padding.bottom;
      let barCount = values.length;
      let barGap = Math.max(4, chartW * 0.02);
      let barW = (chartW - barGap * (barCount + 1)) / barCount;

      // Gradient for bars
      let gradient = ctx.createLinearGradient(0, h - padding.bottom, 0, padding.top);
      gradient.addColorStop(0, '#6366f1');
      gradient.addColorStop(1, '#a78bfa');

      // Draw bars
      values.forEach((v, i) => {
        let barH = Math.max(2, (v / max) * chartH);
        let x = padding.left + barGap + i * (barW + barGap);
        let y = h - padding.bottom - barH;

        // Bar with rounded top
        let radius = Math.min(4, barW / 2);
        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.moveTo(x, y + radius);
        ctx.arcTo(x, y, x + radius, y, radius);
        ctx.arcTo(x + barW, y, x + barW, y + radius, radius);
        ctx.lineTo(x + barW, h - padding.bottom);
        ctx.lineTo(x, h - padding.bottom);
        ctx.closePath();
        ctx.fill();

        // Value label above bar (show only if enough space)
        if (barH > 20 || barCount <= 14) {
          ctx.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--color-fg-2').trim() || '#888';
          ctx.font = 'bold 10px -apple-system, BlinkMacSystemFont, sans-serif';
          ctx.textAlign = 'center';
          ctx.textBaseline = 'bottom';
          ctx.fillText(v, x + barW / 2, y - 4);
        }

        // Date label below bar, rotate if too many
        ctx.fillStyle = getComputedStyle(document.documentElement).getPropertyValue('--color-fg-3').trim() || '#666';
        if (barCount > 14) {
          ctx.save();
          ctx.translate(x + barW / 2, h - padding.bottom + 24);
          ctx.rotate(-Math.PI / 4);
          ctx.font = '9px -apple-system, BlinkMacSystemFont, sans-serif';
          ctx.textAlign = 'right';
          ctx.textBaseline = 'top';
          ctx.fillText(labels[i] || '#' + (i + 1), 0, 0);
          ctx.restore();
        } else {
          ctx.font = '10px -apple-system, BlinkMacSystemFont, sans-serif';
          ctx.textBaseline = 'top';
          ctx.textAlign = 'center';
          ctx.fillText(labels[i] || '#' + (i + 1), x + barW / 2, h - padding.bottom + 6);
        }
      });
    }
  };
}

function overviewStats() {
  return {
    stats: null,
    logs: null,
    copied: false,
    baseUrl: (window.location.protocol + '//' + window.location.hostname + (window.location.port ? ':' + window.location.port : '') + '/api/v1'),

    async fetchData() {
      try {
        let r = await fetch('/api/overview-stats');
        if (r.ok) {
          let data = await r.json();
          this.stats = data.data || data;
          await this.$nextTick();
          this.renderVolumeChart();
          this.renderCacheDonut();
        }
      } catch(e) { console.error('Stats fetch failed', e); }

      try {
        let r = await fetch('/api/dashboard/logs?limit=10');
        if (r.ok) { this.logs = await r.json(); }
      } catch(e) { console.error('Logs fetch failed', e); }

      setTimeout(() => this.fetchData(), 30000);
    },

    async copyUrl() {
      try {
        await navigator.clipboard.writeText(this.baseUrl);
        this.copied = true;
        setTimeout(() => { this.copied = false; }, 2000);
      } catch(e) {
        let inp = document.createElement('input');
        inp.value = this.baseUrl;
        document.body.appendChild(inp);
        inp.select();
        document.execCommand('copy');
        document.body.removeChild(inp);
        this.copied = true;
        setTimeout(() => { this.copied = false; }, 2000);
      }
    },

    renderVolumeChart() {
      let canvas = document.getElementById('volume-chart');
      if (!canvas || !this.stats?.requestVolume) return;
      let ctx = canvas.getContext('2d');
      let values = this.stats.requestVolume;
      let days = ['Mon','Tue','Wed','Thu','Fri','Sat','Sun'];
      let dpr = window.devicePixelRatio || 1;
      let rect = canvas.parentElement.getBoundingClientRect();
      let w = rect.width || canvas.offsetWidth;
      let h = 200;
      canvas.width = w * dpr;
      canvas.height = h * dpr;
      canvas.style.width = w + 'px';
      canvas.style.height = h + 'px';
      ctx.scale(dpr, dpr);
      ctx.clearRect(0,0,w,h);
      let max = Math.max(...values, 1);
      let pad = {top:35, bottom:30, left:10, right:10};
      let cw = w - pad.left - pad.right;
      let ch = h - pad.top - pad.bottom;
      let barCount = values.length;
      let gap = Math.max(8, cw * 0.03);
      let bw = (cw - gap * (barCount + 1)) / barCount;
      let primaryColor = getComputedStyle(document.documentElement).getPropertyValue('--primary').trim() || '#3c50e0';
      let grad = ctx.createLinearGradient(0, h-pad.bottom, 0, pad.top);
      grad.addColorStop(0, primaryColor);
      grad.addColorStop(1, '#6366f1');
      let isDark = document.documentElement.getAttribute('data-theme') !== 'light';
      let lc = isDark ? '#9095a8' : '#64748b';
      let ld = isDark ? '#656b7c' : '#94a3b8';
      values.forEach((v,i) => {
        let bh = Math.max(2, (v/max)*ch);
        let x = pad.left + gap + i*(bw+gap);
        let y = h - pad.bottom - bh;
        let r = Math.max(0, Math.min(4, bw/2));
        ctx.fillStyle = grad;
        if (r > 0 && bw > 4) {
          ctx.beginPath();
          ctx.moveTo(x, y+r);
          ctx.arcTo(x,y,x+r,y,r);
          ctx.arcTo(x+bw,y,x+bw,y+r,r);
          ctx.lineTo(x+bw, h-pad.bottom);
          ctx.lineTo(x, h-pad.bottom);
          ctx.closePath();
          ctx.fill();
        } else {
          ctx.fillRect(x, y, Math.max(1, bw), Math.max(0, bh));
        }
        ctx.fillStyle = lc;
        ctx.font = 'bold 11px Inter, system-ui, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'bottom';
        ctx.fillText(v, x+bw/2, y-4);
        ctx.fillStyle = ld;
        ctx.font = '10px Inter, system-ui, sans-serif';
        ctx.textBaseline = 'top';
        ctx.fillText(days[i]||'D'+(i+1), x+bw/2, h-pad.bottom+6);
      });
    },

    renderCacheDonut() {
      let canvas = document.getElementById('cache-donut');
      if (!canvas || !this.stats) return;
      let ctx = canvas.getContext('2d');
      let primaryColor = getComputedStyle(document.documentElement).getPropertyValue('--primary').trim() || '#3c50e0';
      let dpr = window.devicePixelRatio || 1;
      let size = 150;
      let cx = size/2, cy = size/2;
      let outerR = size/2 - 6;
      let innerR = outerR * 0.62;
      canvas.width = size * dpr;
      canvas.height = size * dpr;
      canvas.style.width = size + 'px';
      canvas.style.height = size + 'px';
      ctx.scale(dpr, dpr);
      ctx.clearRect(0,0,size,size);
      let total = this.stats.totalRequests || 1;
      let cached = this.stats.cachedRequests || 0;
      let hitRate = (cached/total)*100;
      let isDark = document.documentElement.getAttribute('data-theme') !== 'light';
      let sa = -Math.PI/2;
      let ha = (hitRate/100)*2*Math.PI;
      ctx.beginPath();
      ctx.arc(cx,cy,outerR,sa,sa+ha);
      ctx.arc(cx,cy,innerR,sa+ha,sa,true);
      ctx.closePath();
      ctx.fillStyle = primaryColor;
      ctx.fill();
      if (100-hitRate > 0) {
        let ma = ((100-hitRate)/100)*2*Math.PI;
        ctx.beginPath();
        ctx.arc(cx,cy,outerR,sa+ha,sa+ha+ma);
        ctx.arc(cx,cy,innerR,sa+ha+ma,sa+ha,true);
        ctx.closePath();
        ctx.fillStyle = isDark ? '#2a2e3a' : '#e9ecef';
        ctx.fill();
      }
    }
  };
}

// formatContent: renders code blocks, inline code, and bold in assistant messages
window.formatContent = function(content, isUser) {
  if (isUser) return escapeHtml(content);
  if (!content) return '';

  // Split by code blocks (``` ... ```)
  let parts = content.split(/(```[\s\S]*?```)/g);
  return parts.map(function(part) {
    if (part.startsWith('```') && part.endsWith('```')) {
      let inner = part.slice(3, -3);
      let lines = inner.split('\n');
      let lang = lines[0].trim();
      let code = (lang && !lang.includes(' ')) ? lines.slice(1).join('\n') : lines.join('\n');
      let langTag = '';
      if (lang && !lang.includes(' ')) {
        langTag = '<span class="lang-tag">' + escapeHtml(lang) + '</span>';
        code = lines.slice(1).join('\n');
      }
      return '<pre>' + langTag + '<code>' + escapeHtml(code.trimEnd()) + '</code></pre>';
    }
    // Inline code and bold
    let result = escapeHtml(part);
    result = result.replace(/`([^`]+)`/g, '<code>$1</code>');
    result = result.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    return result;
  }).join('');
};

function escapeHtml(str) {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// Auto-scroll chat to bottom when messages or streamText change
(function() {
  var observer = new MutationObserver(function() {
    var el = document.getElementById('pg-chat-scroll');
    if (el) el.scrollTop = el.scrollHeight;
  });
  var check = setInterval(function() {
    var el = document.getElementById('pg-chat-scroll');
    if (el) {
      observer.observe(el, { childList: true, subtree: true, characterData: true });
      clearInterval(check);
    }
  }, 100);
})();

function pluginsPage() {
  return {
    // ── state ──
    activeTab: 'my',
    installedPlugins: null,
    storePlugins: null,
    stats: { installed: 0, active: 0, available: 0 },
    // Store filters
    storeCategories: [
      { key: 'all', label: 'All' },
      { key: 'utility', label: '🛠 Utility' },
      { key: 'moderation', label: '🛡 Moderation' },
      { key: 'logging', label: '📋 Logging' },
      { key: 'translation', label: '🌐 Translation' },
      { key: 'cache', label: '⚡ Cache' },
      { key: 'webhook', label: '🔗 Webhook' },
      { key: 'other', label: '📦 Other' },
    ],
    storeActiveCategory: 'all',
    // AI generate
    aiPrompt: '',
    aiName: '',
    aiLanguage: 'javascript',
    aiGenerating: false,
    aiResult: null,
    aiResultName: '',
    aiInstalling: false,
    toasts: [],

    addToast: function(type, message) {
      this.toasts.push({ type: type, message: message });
    },
  };
}

(function() {
  function getData() {
    var el = document.querySelector('[x-data]');
    return el ? Alpine.$data(el) : null;
  }

  function updateStats() {
    var d = getData(); if (!d) return;
    var installed = (d.installedPlugins || []).length;
    var active = (d.installedPlugins || []).filter(function(p){ return p.enabled; }).length;
    var available = (d.storePlugins|| []).length;
    d.stats = { installed: installed, active: active, available: available };
  }

  window.fetchStats = function() { updateStats(); };

  window.fetchPlugins = async function() {
    var d = getData(); if (!d) { setTimeout(fetchPlugins, 100); return; }
    try {
      var r = await fetch('/api/plugins');
      if (r.ok) {
        var data = await r.json();
        d.installedPlugins = Array.isArray(data) ? data : (data.data || []);
        updateStats();
      }
    } catch(e) { console.error('Fetch plugins failed', e); }
  };

  window.fetchStore = async function() {
    var d = getData(); if (!d) { setTimeout(fetchStore, 100); return; }
    try {
      var r = await fetch('/api/plugins/store');
      if (r.ok) {
        var data = await r.json();
        var items = Array.isArray(data) ? data : (data.data || []);
        // Mark installed plugins
        var installedIds = (d.installedPlugins || []).map(function(p){ return p.name || p.id; });
        items.forEach(function(p) {
          p.installed = installedIds.indexOf(p.name) !== -1;
          p.installing = false;
        });
        d.storePlugins = items;
        updateStats();
      }
    } catch(e) { console.error('Fetch store failed', e); }
  };

  window.gs = {
    // ── category icon ──
    categoryIcon: function(cat) {
      var icons = {
        'utility': '🛠',
        'moderation': '🛡',
        'logging': '📋',
        'translation': '🌐',
        'cache': '⚡',
        'webhook': '🔗',
        'other': '📦',
      };
      return icons[cat] || '🧩';
    },

    // ── toggle ──
    togglePlugin: async function(btn) {
      var d = getData(); if (!d) return;
      var row = btn.closest('tr');
      var idx = Array.from(row.parentNode.children).indexOf(row);
      var p = d.installedPlugins[idx];
      if (!p || !p.id) return;
      try {
        var r = await fetch('/api/plugins/toggle', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id: p.id }),
        });
        if (r.ok) {
          var data = await r.json();
          p.enabled = data.enabled !== undefined ? data.enabled : !p.enabled;
          d.addToast('success', p.enabled ? '"' + p.name + '" enabled.' : '"' + p.name + '" disabled.');
          updateStats();
        }
      } catch(e) { console.error('Toggle plugin failed', e); }
    },

    // ── delete ──
    deletePlugin: async function(btn) {
      var d = getData(); if (!d) return;
      var row = btn.closest('tr');
      var idx = Array.from(row.parentNode.children).indexOf(row);
      var p = d.installedPlugins[idx];
      if (!p || !p.id) return;
      if (!confirm('Delete plugin "' + p.name + '"? This cannot be undone.')) return;
      try {
        var r = await fetch('/api/plugins?id=' + encodeURIComponent(p.id), { method: 'DELETE' });
        if (r.ok) {
          d.installedPlugins = d.installedPlugins.filter(function(pl){ return pl.id !== p.id; });
          d.addToast('success', 'Plugin "' + p.name + '" deleted.');
          updateStats();
          // Refresh store installed status
          window.fetchStore();
        }
      } catch(e) { console.error('Delete plugin failed', e); }
    },

    // ── install from store ──
    installPlugin: async function(btn) {
      var d = getData(); if (!d) return;
      var card = btn.closest('.card');
      if (!card) return;
      var grid = card.closest('[style*="grid"]');
      if (!grid) return;
      var cards = grid.querySelectorAll(':scope > .card');
      var idx = -1;
      for (var i = 0; i < cards.length; i++) {
        if (cards[i] === card) { idx = i; break; }
      }
      if (idx < 0) return;
      var p = d.storePlugins[idx];
      if (!p || p.installed) return;

      p.installing = true;
      try {
        var r = await fetch('/api/plugins', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            action: 'install',
            name: p.name,
            version: p.version || '0.0.0',
            description: p.description || '',
            category: p.category || '',
            author: p.author || '',
          }),
        });
        if (r.ok) {
          p.installed = true;
          p.installing = false;
          await fetchPlugins();
          updateStats();
          d.addToast('success', 'Plugin "' + p.name + '" installed!');
        } else {
          p.installing = false;
          d.addToast('error', 'Failed to install "' + p.name + '".');
        }
      } catch(e) {
        p.installing = false;
        d.addToast('error', 'Network error installing "' + p.name + '".');
      }
    },

    // ── AI generate ──
    aiGenerate: async function() {
      var d = getData(); if (!d) return;
      if (!d.aiPrompt.trim()) return;
      d.aiGenerating = true;
      d.aiResult = null;
      try {
        var r = await fetch('/api/plugins/generate', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            prompt: d.aiPrompt.trim(),
            name: d.aiName.trim() || 'ai-generated-plugin',
            language: d.aiLanguage,
          }),
        });
        if (r.ok) {
          var data = await r.json();
          d.aiResult = data.code || data.result || '';
          d.aiResultName = data.name || d.aiName || 'ai-generated-plugin';
        } else {
          var err = await r.text().catch(function(){ return ''; });
          d.addToast('error', 'Generation failed: ' + (err || 'Unknown error'));
        }
      } catch(e) {
        d.addToast('error', 'Network error during generation.');
      } finally {
        d.aiGenerating = false;
      }
    },

    // ── install generated ──
    installGenerated: async function() {
      var d = getData(); if (!d) return;
      if (!d.aiResult) return;
      d.aiInstalling = true;
      try {
        var r = await fetch('/api/plugins', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            action: 'install-generated',
            name: d.aiResultName,
            code: d.aiResult,
            language: d.aiLanguage,
          }),
        });
        if (r.ok) {
          d.addToast('success', 'Plugin "' + d.aiResultName + '" installed!');
          await fetchPlugins();
          updateStats();
        } else {
          var err = await r.text().catch(function(){ return ''; });
          d.addToast('error', 'Install failed: ' + (err || 'Unknown error'));
        }
      } catch(e) {
        d.addToast('error', 'Network error during install.');
      } finally {
        d.aiInstalling = false;
      }
    },
  };
})();

function routingPage() {
  return {
    // ── state ──
    combos: null,
    aliases: null,
    connections: [],
    stats: { combos: 0, strategy: '—', aliases: 0 },
    // Load balancer
    lbStrategy: 'priority',
    lbSaving: false,
    lbSaved: false,
    // Combo modal
    comboModal: false,
    comboEditMode: false,
    comboEditId: null,
    comboSaving: false,
    comboForm: { name: '', description: '', strategy: 'priority', sticky_limit: '3', is_active: true, models: [] },
    // Alias modal
    aliasModal: false,
    aliasEditMode: false,
    aliasEditId: null,
    aliasSaving: false,
    aliasForm: { name: '', target: '', provider: '' },
    toasts: [],

    addToast: function(type, message) {
      this.toasts.push({ type: type, message: message });
    },

    addModelEntry: function() {
      this.comboForm.models.push({ model: '', account: '' });
    },

    setStrategy: function(strat) { this.lbStrategy = strat; },

    saveLoadBalancer: async function() {
      this.lbSaving = true;
      this.lbSaved = false;
      try {
        var r = await fetch('/api/load-balancer', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ strategy: this.lbStrategy }),
        });
        if (r.ok) {
          this.lbSaved = true;
          window.rsFetchStats();
          var self = this;
          setTimeout(function() { self.lbSaved = false; }, 3000);
        } else {
          this.addToast('error', 'Failed to save strategy.');
        }
      } catch(e) {
        this.addToast('error', 'Network error.');
      } finally {
        this.lbSaving = false;
      }
    },
  };
}

(function() {
  function getData() {
    var el = document.querySelector('[x-data]');
    return el ? Alpine.$data(el) : null;
  }

  window.rsFetchStats = async function() {
    var d = getData(); if (!d) { setTimeout(rsFetchStats, 100); return; }
    var combos = (d.combos || []).length;
    var aliases = (d.aliases || []).length;
    d.stats = { combos: combos, strategy: d.lbStrategy || 'priority', aliases: aliases };
  };

  window.rsFetchConnections = async function() {
    var d = getData(); if (!d) { setTimeout(rsFetchConnections, 100); return; }
    try {
      var r = await fetch('/api/connections');
      if (r.ok) {
        var data = await r.json();
        d.connections = Array.isArray(data) ? data : (data.data || []);
      }
    } catch(e) { console.error('Fetch connections failed', e); }
  };

  window.rsFetchCombos = async function() {
    var d = getData(); if (!d) { setTimeout(rsFetchCombos, 100); return; }
    try {
      var r = await fetch('/api/combos');
      if (r.ok) {
        var data = await r.json();
        d.combos = Array.isArray(data) ? data : (data.data || []);
        window.rsFetchStats();
      }
    } catch(e) { console.error('Fetch combos failed', e); }
  };

  window.rsFetchLoadBalancer = async function() {
    var d = getData(); if (!d) { setTimeout(rsFetchLoadBalancer, 100); return; }
    try {
      var r = await fetch('/api/load-balancer');
      if (r.ok) {
        var data = await r.json();
        var lb = data.data || data;
        d.lbStrategy = lb.strategy || lb.default || 'priority';
        window.rsFetchStats();
      }
    } catch(e) { console.error('Fetch load balancer failed', e); }
  };

  window.rsFetchAliases = async function() {
    var d = getData(); if (!d) { setTimeout(rsFetchAliases, 100); return; }
    try {
      var r = await fetch('/api/aliases');
      if (r.ok) {
        var data = await r.json();
        var raw = data.data || data;
        if (Array.isArray(raw)) {
          d.aliases = raw;
        } else if (raw && typeof raw === 'object') {
          // Transform map {alias: {model: "..."}} → array [{name, target, provider}]
          var arr = [];
          Object.keys(raw).forEach(function(k) {
            var v = raw[k];
            if (v && typeof v === 'object') {
              arr.push({ name: k, target: v.model || v.target || '', provider: v.provider || '' });
            } else if (typeof v === 'string') {
              arr.push({ name: k, target: v, provider: '' });
            }
          });
          d.aliases = arr;
        } else {
          d.aliases = [];
        }
        window.rsFetchStats();
      }
    } catch(e) { console.error('Fetch aliases failed', e); }
  };

  window.rs = {
    // ── combo modal ──
    openNewCombo: function() {
      var d = getData(); if (!d) return;
      d.comboModal = true;
      d.comboEditMode = false;
      d.comboEditId = null;
      d.comboForm = { name: '', description: '', strategy: 'priority', sticky_limit: '3', is_active: true, models: [] };
      window.rsFetchConnections();
    },

    editCombo: function(btn) {
      var d = getData(); if (!d) return;
      var row = btn.closest('tr');
      var rows = Array.from(row.parentNode.querySelectorAll('tr'));
      var idx = rows.indexOf(row);
      var combo = d.combos[idx];
      if (!combo) return;
      d.comboModal = true;
      d.comboEditMode = true;
      d.comboEditId = combo.id;
      // Convert models array to entry objects with account
      var models = Array.isArray(combo.models)
        ? combo.models.map(function(m) {
            if (typeof m === 'string') return { model: m, account: '' };
            return { model: m.model || m.name || '', account: m.account || m.provider_id || '' };
          })
        : [];
      d.comboForm = {
        name: combo.name || '',
        description: combo.description || '',
        strategy: combo.strategy || 'priority',
        sticky_limit: combo.sticky_limit != null ? String(combo.sticky_limit) : '3',
        is_active: combo.is_active !== false,
        models: models,
      };
      window.rsFetchConnections();
    },

    closeComboModal: function() {
      var d = getData(); if (!d) return;
      d.comboModal = false;
      d.comboEditMode = false;
      d.comboEditId = null;
    },

    saveCombo: async function() {
      var d = getData(); if (!d) return;
      if (!d.comboForm.name.trim()) return;
      d.comboSaving = true;
      try {
        var modelsPayload = d.comboForm.models.map(function(e) {
          return { model: e.model, account: e.account || null };
        });
        var payload = {
          name: d.comboForm.name.trim(),
          description: d.comboForm.description.trim(),
          strategy: d.comboForm.strategy,
          sticky_limit: parseInt(d.comboForm.sticky_limit) || 3,
          is_active: d.comboForm.is_active,
          models: modelsPayload,
        };

        var method = d.comboEditMode ? 'PUT' : 'POST';
        var url = '/api/combos';
        if (d.comboEditMode && d.comboEditId) {
          url += '?id=' + encodeURIComponent(d.comboEditId);
        }

        var r = await fetch(url, {
          method: method,
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });

        if (r.ok) {
          rs.closeComboModal();
          await rsFetchCombos();
          d.addToast('success', d.comboEditMode ? 'Combo updated.' : 'Combo created.');
        } else {
          var err = await r.text().catch(function(){ return ''; });
          d.addToast('error', 'Save failed: ' + (err || 'HTTP ' + r.status));
        }
      } catch(e) {
        console.error('Save combo failed', e);
        d.addToast('error', 'Network error.');
      } finally {
        d.comboSaving = false;
      }
    },

    deleteCombo: async function(btn) {
      var d = getData(); if (!d) return;
      var row = btn.closest('tr');
      var rows = Array.from(row.parentNode.querySelectorAll('tr'));
      var idx = rows.indexOf(row);
      var combo = d.combos[idx];
      if (!combo || !combo.id) return;
      if (!confirm('Delete combo "' + combo.name + '"? This cannot be undone.')) return;
      try {
        var r = await fetch('/api/combos?id=' + encodeURIComponent(combo.id), { method: 'DELETE' });
        if (r.ok) {
          d.combos = d.combos.filter(function(c){ return c.id !== combo.id; });
          d.addToast('success', 'Combo deleted.');
          window.rsFetchStats();
        } else {
          d.addToast('error', 'Delete failed.');
        }
      } catch(e) {
        d.addToast('error', 'Network error.');
      }
    },

    // ── load balancer ──
    setStrategy: function(strat) {
      var d = getData(); if (!d) return;
      d.lbStrategy = strat;
    },

    removeModelEntry: function(btn) {
      var d = getData(); if (!d) return;
      var entryDiv = btn.closest('div[style*="flex"]');
      if (!entryDiv) return;
      var container = entryDiv.parentElement;
      if (!container) return;
      var allEntries = container.querySelectorAll(':scope > div[style*="flex"]');
      var idx = Array.from(allEntries).indexOf(entryDiv);
      if (idx >= 0) d.comboForm.models.splice(idx, 1);
    },

    saveLoadBalancer: async function() {
      var d = getData(); if (!d) return;
      d.lbSaving = true;
      d.lbSaved = false;
      try {
        var r = await fetch('/api/load-balancer', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ strategy: d.lbStrategy }),
        });
        if (r.ok) {
          d.lbSaved = true;
          window.rsFetchStats();
          setTimeout(function() { d.lbSaved = false; }, 3000);
        } else {
          d.addToast('error', 'Failed to save strategy.');
        }
      } catch(e) {
        d.addToast('error', 'Network error.');
      } finally {
        d.lbSaving = false;
      }
    },

    // ── alias modal ──
    openNewAlias: function() {
      var d = getData(); if (!d) return;
      d.aliasModal = true;
      d.aliasEditMode = false;
      d.aliasEditId = null;
      d.aliasForm = { name: '', target: '', provider: '' };
      window.rsFetchConnections();
    },

    editAlias: function(btn) {
      var d = getData(); if (!d) return;
      var row = btn.closest('tr');
      var rows = Array.from(row.parentNode.querySelectorAll('tr'));
      var idx = rows.indexOf(row);
      var alias = d.aliases[idx];
      if (!alias) return;
      d.aliasModal = true;
      d.aliasEditMode = true;
      d.aliasEditId = alias.id || alias.name;
      d.aliasForm = {
        name: alias.name || alias.alias || '',
        target: alias.target || alias.model || '',
        provider: alias.provider || alias.provider_id || '',
      };
      window.rsFetchConnections();
    },

    closeAliasModal: function() {
      var d = getData(); if (!d) return;
      d.aliasModal = false;
      d.aliasEditMode = false;
      d.aliasEditId = null;
    },

    saveAlias: async function() {
      var d = getData(); if (!d) return;
      if (!d.aliasForm.name.trim() || !d.aliasForm.target.trim()) return;
      d.aliasSaving = true;
      try {
        var payload = {
          name: d.aliasForm.name.trim(),
          target: d.aliasForm.target.trim(),
          provider: d.aliasForm.provider || null,
        };

        var url = '/api/aliases';
        if (d.aliasEditMode && d.aliasEditId) {
          url += '?id=' + encodeURIComponent(d.aliasEditId);
        }

        var r = await fetch(url, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });

        if (r.ok) {
          rs.closeAliasModal();
          await rsFetchAliases();
          d.addToast('success', d.aliasEditMode ? 'Alias updated.' : 'Alias created.');
        } else {
          var err = await r.text().catch(function(){ return ''; });
          d.addToast('error', 'Save failed: ' + (err || 'HTTP ' + r.status));
        }
      } catch(e) {
        console.error('Save alias failed', e);
        d.addToast('error', 'Network error.');
      } finally {
        d.aliasSaving = false;
      }
    },

    deleteAlias: async function(btn) {
      var d = getData(); if (!d) return;
      var row = btn.closest('tr');
      var rows = Array.from(row.parentNode.querySelectorAll('tr'));
      var idx = rows.indexOf(row);
      var alias = d.aliases[idx];
      if (!alias) return;
      var id = alias.id || alias.name;
      if (!id) return;
      if (!confirm('Delete alias "' + (alias.name || alias.alias) + '"?')) return;
      try {
        var r = await fetch('/api/aliases?id=' + encodeURIComponent(id), { method: 'DELETE' });
        if (r.ok) {
          d.aliases = d.aliases.filter(function(a){ return (a.id || a.name) !== id; });
          d.addToast('success', 'Alias deleted.');
          window.rsFetchStats();
        } else {
          d.addToast('error', 'Delete failed.');
        }
      } catch(e) {
        d.addToast('error', 'Network error.');
      }
    },
  };
})();

(function() {
    function getData() {
      var el = document.querySelector('[x-data]');
      return el ? Alpine.$data(el) : null;
    }

    // Debounced auto-save: sends PUT /api/settings with the changed key:value
    var saveTimers = {};

    function autoSave(key, value) {
      var d = getData(); if (!d) return;
      // Clear any existing timer for this key
      if (saveTimers[key]) clearTimeout(saveTimers[key]);

      saveTimers[key] = setTimeout(async function() {
        try {
          // Build a flat payload for this field
          var payload = {};
          // Support dot-notation keys for nested objects
          var keys = key.split('.');
          if (keys.length === 1) {
            payload[key] = value;
          } else {
            // For nested keys like 'token_optimization.rtk_enabled', send the whole section
            var section = keys[0];
            payload[section] = JSON.parse(JSON.stringify(d.settings[section]));
          }

          var r = await fetch('/api/settings', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });

          if (r.ok) {
            d.savedFields[key] = true;
            setTimeout(function() { d.savedFields[key] = false; }, 2000);
          } else {
            console.error('Auto-save failed for', key);
          }
        } catch(e) {
          console.error('Auto-save error for', key, e);
        }
      }, 600); // 600ms debounce
    }

    // Expose autoSave globally
    window.autoSave = autoSave;

    window.fetchSettings = async function() {
      var d = getData();
      if (!d) { setTimeout(fetchSettings, 100); return; }
      try {
        var r = await fetch('/api/settings');
        if (r.ok) {
          var data = await r.json();
          if (data && typeof data === 'object') {
            // Merge remote settings into local defaults
            if (data.token_optimization) {
              Object.assign(d.settings.token_optimization, data.token_optimization);
            }
            if (data.api_security) {
              Object.assign(d.settings.api_security, data.api_security);
            }
            if (data.caching) {
              Object.assign(d.settings.caching, data.caching);
            }
            if (data.embedding_cache) {
              Object.assign(d.settings.embedding_cache, data.embedding_cache);
            }
            if (data.thinking_mode !== undefined) {
              d.settings.thinking_mode = data.thinking_mode;
            }
            if (data.ai_agent_model !== undefined) {
              d.settings.ai_agent_model = data.ai_agent_model;
            }
            if (data.models && Array.isArray(data.models)) {
              d.models = data.models;
            }
          }
        }
      } catch(e) { console.error('Settings fetch failed', e); }
    };
  })();

function teamsData() {
  return {
    teams: null,
    selectedTeam: null,
    showForm: false,
    showAddMember: false,
    newTeam: { name: '', description: '' },
    newMember: { email: '', role: 'member' },
    toasts: [],

    get totalMembers() {
      return (this.teams || []).reduce((sum, t) => sum + (t.members?.length || 0), 0);
    },
    get activeTeams() {
      return (this.teams || []).filter(t => t.active !== false).length;
    },

    async fetchTeams() {
      try {
        let r = await fetch('/api/teams');
        if (r.ok) {
          let data = await r.json();
          this.teams = Array.isArray(data) ? data : (data.data || []);
        }
      } catch(e) { console.error('Teams fetch failed', e); }
    },

    async createTeam() {
      if (!this.newTeam.name.trim()) return;
      try {
        let r = await fetch('/api/teams', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(this.newTeam)
        });
        if (r.ok) {
          let data = await r.json();
          let newTeam = Array.isArray(data) ? data[data.length-1] : data;
          if (!this.teams) this.teams = [];
          this.teams.unshift(newTeam);
          this.showForm = false;
          this.newTeam = { name: '', description: '' };
          this.showToast('success', 'Team created successfully');
        }
      } catch(e) { console.error('Create team failed', e); this.showToast('error', 'Failed to create team'); }
    },

    async deleteTeam(team, idx) {
      if (!confirm('Delete team "' + team.name + '"? This cannot be undone.')) return;
      try {
        let r = await fetch('/api/teams?id=' + team.id, { method: 'DELETE' });
        if (r.ok) {
          this.teams.splice(idx, 1);
          if (this.selectedTeam?.id === team.id) this.selectedTeam = null;
          this.showToast('success', 'Team deleted');
        }
      } catch(e) { console.error('Delete team failed', e); this.showToast('error', 'Failed to delete team'); }
    },

    selectTeam(team) {
      this.selectedTeam = team;
      this.showAddMember = false;
    },

    async addMember() {
      if (!this.newMember.email.trim() || !this.selectedTeam) return;
      try {
        let r = await fetch('/api/teams/' + this.selectedTeam.id + '/members', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(this.newMember)
        });
        if (r.ok) {
          let data = await r.json();
          if (!this.selectedTeam.members) this.selectedTeam.members = [];
          this.selectedTeam.members.push(data);
          this.newMember = { email: '', role: 'member' };
          this.showAddMember = false;
          this.showToast('success', 'Member added');
        }
      } catch(e) { console.error('Add member failed', e); this.showToast('error', 'Failed to add member'); }
    },

    async removeMember(member) {
      if (!confirm('Remove ' + (member.name || member.email) + ' from this team?')) return;
      try {
        let r = await fetch('/api/teams/' + this.selectedTeam.id + '/members?email=' + encodeURIComponent(member.email), { method: 'DELETE' });
        if (r.ok && this.selectedTeam.members) {
          let idx = this.selectedTeam.members.indexOf(member);
          if (idx > -1) this.selectedTeam.members.splice(idx, 1);
          this.showToast('success', 'Member removed');
        }
      } catch(e) { console.error('Remove member failed', e); this.showToast('error', 'Failed to remove member'); }
    },

    getInitials(name) {
      if (!name) return '?';
      return name.split(' ').map(n => n.charAt(0)).join('').substring(0, 2).toUpperCase();
    },

    avatarColor(email) {
      if (!email) return '#3c50e0';
      let hash = 0;
      for (let i = 0; i < email.length; i++) {
        hash = email.charCodeAt(i) + ((hash << 5) - hash);
      }
      let colors = ['#3c50e0', '#8b5cf6', '#ec4899', '#f59e0b', '#10b981', '#06b6d4', '#6366f1'];
      return colors[Math.abs(hash) % colors.length];
    },

    showToast(type, message) {
      let toast = { type, message };
      this.toasts.push(toast);
      setTimeout(() => {
        let idx = this.toasts.indexOf(toast);
        if (idx > -1) this.toasts.splice(idx, 1);
      }, 3000);
    }
  };
}

// Color palette matching Node.js
var COLORS = ['var(--primary)', 'var(--success)', 'var(--warning)', '#8b5cf6', '#ec4899', '#06b6d4'];

function shortModel(name) {
  if (!name) return 'unknown';
  var parts = name.split('/');
  return parts[parts.length - 1];
}

function fmtNum(n) {
  n = n || 0;
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
  if (n >= 1000) return Math.round(n / 1000) + 'K';
  return String(n);
}

function fmtTotal(providers) {
  if (!providers || !providers.length) return '0';
  var total = providers.reduce(function(s, p) { return s + (p.tokens || 0); }, 0);
  return fmtNum(total);
}

// Expose max values as Alpine computes by hooking into the data
function fetchData() {
  var self = this;
  fetch('/api/usage')
    .then(function(r) { return r.json(); })
    .then(function(d) {
      // Go API returns {providers, models, daily} directly
      self.providers = (d.providers || []).map(function(p) { return { provider: p.provider, requests: p.requests, tokens: p.tokens || 0 }; });
      self.models = (d.models || []).map(function(m) { return { model: m.model, requests: m.requests, tokens: m.tokens || 0 }; });
      self.daily = (d.daily || []).map(function(d) { return { date: d.date, requests: d.requests, tokens: d.tokens || 0 }; });

      // Compute max values
      self.maxProvider = Math.max.apply(null, (self.providers || []).map(function(p) { return p.tokens || 0; }).concat([1]));
      self.maxModel = Math.max.apply(null, (self.models || []).map(function(m) { return m.tokens || 0; }).concat([1]));
      self.maxDaily = Math.max.apply(null, (self.daily || []).map(function(d) { return d.tokens || 0; }).concat([1]));

      self.loading = false;
    })
    .catch(function(e) {
      console.error('Usage fetch failed', e);
      self.loading = false;
    })
    .then(function() {
      setTimeout(function() { fetchData.call(self); }, 30000);
    });
}

(function() {
    function getData() {
      var el = document.querySelector('[x-data]');
      return el ? Alpine.$data(el) : null;
    }

    function computeStats() {
      var d = getData(); if (!d || !d.users) return;
      d.stats.total = d.users.length;
      d.stats.admins = d.users.filter(function(u) { return u.role === 'admin'; }).length;
      d.stats.active = d.users.filter(function(u) { return u.is_active; }).length;
      d.stats.inactive = d.stats.total - d.stats.active;
    }

    window.usHandler = {
      createUser: async function() {
        var d = getData(); if (!d) return;
        try {
          var r = await fetch('/api/users', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              username: d.form.username,
              password: d.form.password,
              email: d.form.email,
              role: d.form.role
            })
          });
          if (r.ok) {
            d.form = { username: '', password: '', email: '', role: 'viewer' };
            await fetchUsers();
            d.toasts.push({ type: 'success', message: 'User added successfully' });
          } else {
            d.toasts.push({ type: 'error', message: 'Failed to add user' });
          }
        } catch(e) { console.error('Create user failed', e); }
      },

      toggleUser: async function(user) {
        var d = getData(); if (!d || !user.id) return;
        var newState = !user.is_active;
        try {
          var r = await fetch('/api/users?id=' + user.id, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ is_active: newState })
          });
          if (r.ok) {
            user.is_active = newState;
            computeStats();
            d.toasts.push({ type: 'success', message: 'User ' + (newState ? 'activated' : 'deactivated') });
          }
        } catch(e) { console.error('Toggle user failed', e); }
      },

      deleteUser: async function(user) {
        var d = getData(); if (!d || !user.id) return;
        if (!confirm('Delete user "' + (user.username || user.name || user.email) + '"? This cannot be undone.')) return;
        try {
          var r = await fetch('/api/users?id=' + user.id, { method: 'DELETE' });
          if (r.ok) {
            d.users = d.users.filter(function(u) { return u.id !== user.id; });
            computeStats();
            d.toasts.push({ type: 'success', message: 'User deleted' });
          }
        } catch(e) { console.error('Delete user failed', e); }
      }
    };

    window.fetchUsers = async function() {
      var d = getData();
      if (!d) { setTimeout(fetchUsers, 100); return; }
      try {
        var r = await fetch('/api/users');
        if (r.ok) {
          var data = await r.json();
          d.users = Array.isArray(data) ? data : (data.data || []);
          computeStats();
        }
      } catch(e) { console.error('Users fetch failed', e); }
    };
  })();

(function() {
    function getData() {
      var el = document.querySelector('[x-data]');
      return el ? Alpine.$data(el) : null;
    }

    function computeStats() {
      var d = getData(); if (!d || !d.webhooks) return;
      d.stats.total = d.webhooks.length;
      d.stats.active = d.webhooks.filter(function(w) { return w.is_active; }).length;
      d.stats.failed24h = (d.history || [])
        .filter(function(e) { return e.status_code >= 400; }).length;
    }

    function buildEventsPayload(eventsObj) {
      return Object.keys(eventsObj).filter(function(k) { return eventsObj[k]; });
    }

    window.whHandler = {
      createWebhook: async function() {
        var d = getData(); if (!d) return;
        try {
          var r = await fetch('/api/webhooks', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              name: d.form.name,
              url: d.form.url,
              secret: d.form.secret,
              events: buildEventsPayload(d.form.events)
            })
          });
          if (r.ok) {
            d.form = { name: '', url: '', secret: '', events: { completion: true, error: true, usage: false, moderation: false } };
            await fetchWebhooks();
            d.toasts.push({ type: 'success', message: 'Webhook created successfully' });
          } else {
            d.toasts.push({ type: 'error', message: 'Failed to create webhook' });
          }
        } catch(e) { console.error('Create webhook failed', e); }
      },

      toggleWebhook: async function(wh) {
        var d = getData(); if (!d || !wh.id) return;
        var newState = !wh.is_active;
        try {
          var r = await fetch('/api/webhooks?id=' + wh.id, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ is_active: newState })
          });
          if (r.ok) {
            wh.is_active = newState;
            computeStats();
            d.toasts.push({ type: 'success', message: 'Webhook ' + (newState ? 'activated' : 'deactivated') });
          }
        } catch(e) { console.error('Toggle webhook failed', e); }
      },

      testWebhook: async function(wh) {
        var d = getData(); if (!d || !wh.id) return;
        d.toasts.push({ type: 'success', message: 'Test event sent to "' + wh.name + '"' });
        try {
          var r = await fetch('/api/webhooks/' + wh.id + '/test', { method: 'POST' });
          if (r.ok) {
            await fetchWebhookHistory();
          }
        } catch(e) { console.error('Test webhook failed', e); }
      },

      deleteWebhook: async function(wh) {
        var d = getData(); if (!d || !wh.id) return;
        if (!confirm('Delete webhook "' + wh.name + '"? This cannot be undone.')) return;
        try {
          var r = await fetch('/api/webhooks?id=' + wh.id, { method: 'DELETE' });
          if (r.ok) {
            d.webhooks = d.webhooks.filter(function(w) { return w.id !== wh.id; });
            computeStats();
            d.toasts.push({ type: 'success', message: 'Webhook deleted' });
          }
        } catch(e) { console.error('Delete webhook failed', e); }
      }
    };

    window.fetchWebhooks = async function() {
      var d = getData();
      if (!d) { setTimeout(fetchWebhooks, 100); return; }
      try {
        var r = await fetch('/api/webhooks');
        if (r.ok) {
          var data = await r.json();
          d.webhooks = Array.isArray(data) ? data : (data.data || []);
          computeStats();
        }
      } catch(e) { console.error('Webhooks fetch failed', e); }
    };

    window.fetchWebhookHistory = async function() {
      var d = getData();
      if (!d) return;
      try {
        var r = await fetch('/api/webhooks/history');
        if (r.ok) {
          var data = await r.json();
          d.history = Array.isArray(data) ? data : (data.data || []);
          computeStats();
        }
      } catch(e) { console.error('History fetch failed', e); }
    };

    // Fetch history on init too
    var origInit = fetchWebhooks;
    fetchWebhooks = async function() {
      await origInit();
      await fetchWebhookHistory();
    };
  })();
