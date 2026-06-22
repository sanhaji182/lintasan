<script lang="ts">
  import { onMount } from 'svelte';
  import {
    ArrowRight, Gauge, Globe, LayoutDashboard,
    ShieldCheck, Sparkles, Zap,
    Check, X as XIcon,
    Star, ArrowUpRight, ChevronRight, Layers,
    BarChart3, Cpu, BookOpen
  } from 'lucide-svelte';
  import { api } from '$lib/api';

  let version = $state('v0.x');
  let mounted = $state(false);
  let checkingAuth = $state(true);
  let isAuthenticated = $state(false);

  // Animated metric counters
  let animatedCount = $state({ providers: 0, models: 0, requests: 0, uptime: '—' });
  const targetCount = { providers: 100, models: 350, requests: 1_000_000 };
  let countersStarted = $state(false);

  const features = [
    {
      icon: Globe, title: 'Unified Provider Routing',
      desc: 'Satu endpoint untuk banyak provider dengan fallback cerdas dan failover otomatis.'
    },
    {
      icon: Gauge, title: 'Low-Latency Performance',
      desc: 'Pantau latency, cache hit rate, dan request health dalam dashboard real-time.'
    },
    {
      icon: ShieldCheck, title: 'Secure by Default',
      desc: 'JWT auth, API key controls, audit logs, dan proteksi untuk operasi production.'
    },
    {
      icon: Zap, title: 'Smart Caching',
      desc: 'Semantic cache mengurangi beban provider dan mempercepat response hingga 10x.'
    },
    {
      icon: Sparkles, title: 'Format Translation',
      desc: 'Bridging format lintas provider tanpa ubah client di layer aplikasi.'
    },
    {
      icon: BarChart3, title: 'Cost Optimization',
      desc: 'Tracking pengeluaran per provider, budget limits, dan savings recommendations.'
    }
  ];

  const steps = [
    { step: '01', title: 'Connect Providers', desc: 'Tambah credentials dan aktifkan model discovery dari 100+ provider preset.' },
    { step: '02', title: 'Configure Routing', desc: 'Atur priority, failover chain, caching, dan kebijakan per-model.' },
    { step: '03', title: 'Ship with Confidence', desc: 'Monitor statistik, logs, dan analytics real-time untuk validasi production.' }
  ];

  const comparisons = [
    { aspect: 'Setup time', lintasan: '5 menit', raw: '30+ menit per provider' },
    { aspect: 'Unified endpoint', lintasan: true, raw: false },
    { aspect: 'Smart failover', lintasan: true, raw: false },
    { aspect: 'Response caching', lintasan: true, raw: false },
    { aspect: 'Observability', lintasan: true, raw: false },
    { aspect: 'Cost tracking', lintasan: true, raw: false },
    { aspect: 'API key management', lintasan: true, raw: false },
  ];

  onMount(async () => {
    mounted = true;

    // Fetch version from /health
    try {
      const h = await fetch('/health').then(r => r.ok ? r.json() : null);
      if (h?.version) version = h.version;
    } catch {}

    const token = localStorage.getItem('lintasan_token');
    if (!token) { checkingAuth = false; return; }
    try {
      await api.get('/api/auth/me');
      isAuthenticated = true;
    } catch {
      localStorage.removeItem('lintasan_token');
      localStorage.removeItem('lintasan_user');
    } finally { checkingAuth = false; }

    // Animate counters
    if (!countersStarted) {
      countersStarted = true;
      const dur = 1500;
      const start = performance.now();
      function tick() {
        const el = Math.min((performance.now() - start) / dur, 1);
        const p = 1 - Math.pow(1 - el, 3);
        animatedCount.providers = Math.round(p * targetCount.providers);
        animatedCount.models = Math.round(p * targetCount.models);
        animatedCount.requests = Math.round(p * targetCount.requests);
        if (el < 1) requestAnimationFrame(tick);
        else {
          animatedCount.providers = targetCount.providers;
          animatedCount.models = targetCount.models;
          animatedCount.requests = targetCount.requests;
        }
      }
      requestAnimationFrame(tick);
    }
  });

  const ctaHref = $derived(checkingAuth ? '/login' : (isAuthenticated ? '/dashboard' : '/login'));
  const ctaLabel = $derived(checkingAuth ? 'Loading...' : (isAuthenticated ? 'Go to Dashboard' : 'Get Started'));

  function formatNum(n: number): string {
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(0) + 'M';
    if (n >= 1_000) return (n / 1_000).toFixed(0) + 'K';
    return String(n);
  }
</script>

<svelte:head>
  <title>Lintasan — AI Gateway Management</title>
  <meta name="description" content="Kelola multi-provider LLM routing dengan UI modern, aman, dan siap produksi." />
</svelte:head>

<div class="landing" class:mounted>
  <!-- Floating decorative elements -->
  <div class="deco-glow deco-glow-1"></div>
  <div class="deco-glow deco-glow-2"></div>
  <div class="deco-dots deco-dots-1"></div>
  <div class="deco-dots deco-dots-2"></div>

  <header class="topbar">
    <a class="brand" href="/">
      <span class="brand-mark">L</span>
      <span class="brand-name">Lintasan</span>
    </a>
    <nav class="nav-links">
      <a href="/login" class="nav-link">Sign In</a>
      <a href={ctaHref} class="btn-cta">
        {ctaLabel}
        <ArrowRight size={14} />
      </a>
    </nav>
  </header>

  <main>
    <!-- Hero with gradient background -->
    <section class="hero">
      <div class="hero-bg">
        <div class="hero-bg-glow"></div>
      </div>
      <div class="hero-inner">
        <div class="hero-kicker-wrap">
          <span class="hero-kicker">
            <Zap size={12} />
            AI Gateway Control Plane
          </span>
        </div>
        <h1 class="hero-title">
          Route smarter.<br />
          <span class="hero-title-accent">Ship faster.</span>
        </h1>
        <p class="hero-sub">
          Lintasan mengelola semua koneksi LLM provider kamu — routing, fallback,
          caching, dan observability — dalam satu dashboard yang bersih dan efisien.
        </p>
        <div class="hero-actions">
          <a href={ctaHref} class="btn-hero">
            {ctaLabel}
            <ArrowRight size={16} />
          </a>
          <a href="/dashboard/docs" class="btn-hero-outline">
            <BookOpen size={15} />
            Documentation
          </a>
        </div>
        <div class="hero-badge-row">
          <span class="hero-badge"><Cpu size={12} /> {version}</span>
          <span class="hero-badge"><ShieldCheck size={12} /> Open Source</span>
          <span class="hero-badge"><Zap size={12} /> MIT License</span>
        </div>
      </div>
    </section>

    <!-- Metrics strip with gradient accent -->
    <section class="metrics-strip">
      <div class="metrics-inner">
        <div class="metric-item">
          <div class="metric-val">{formatNum(animatedCount.providers)}+</div>
          <div class="metric-label">Provider Presets</div>
        </div>
        <div class="metric-divider"></div>
        <div class="metric-item">
          <div class="metric-val">{formatNum(animatedCount.models)}+</div>
          <div class="metric-label">Supported Models</div>
        </div>
        <div class="metric-divider"></div>
        <div class="metric-item">
          <div class="metric-val">{formatNum(animatedCount.requests)}+</div>
          <div class="metric-label">Requests / Day</div>
        </div>
        <div class="metric-divider"></div>
        <div class="metric-item">
          <div class="metric-val">99.9%</div>
          <div class="metric-label">Uptime SLA</div>
        </div>
      </div>
    </section>

    <!-- Features -->
    <section class="features">
      <div class="section-header">
        <span class="section-badge">Features</span>
        <h2 class="section-title">Everything you need</h2>
        <p class="section-sub">Tools lengkap untuk operasional AI engineering harian.</p>
      </div>
      <div class="feature-grid">
        {#each features as f, i}
          <article class="feature-card" style="animation: fadeInUp 0.5s ease-out {0.08 * i}s both;">
            <div class="feature-icon"><f.icon size={22} stroke-width={1.5} /></div>
            <h3>{f.title}</h3>
            <p>{f.desc}</p>
          </article>
        {/each}
      </div>
    </section>

    <!-- Comparison table -->
    <section class="compare">
      <div class="section-header">
        <span class="section-badge">Comparison</span>
        <h2 class="section-title">Why Lintasan?</h2>
        <p class="section-sub">See how it stacks up against managing providers directly.</p>
      </div>
      <div class="compare-table-wrap">
        <table class="compare-table">
          <thead>
            <tr>
              <th></th>
              <th class="th-lintasan">
                <span class="th-brand-mark">L</span>
                Lintasan
              </th>
              <th class="th-raw">Raw Providers</th>
            </tr>
          </thead>
          <tbody>
            {#each comparisons as cmp}
              <tr>
                <td class="cmp-aspect">{cmp.aspect}</td>
                <td class="cmp-cell">
                  {#if typeof cmp.lintasan === 'boolean'}
                    {#if cmp.lintasan}
                      <span class="cmp-yes"><Check size={14} /></span>
                    {:else}
                      <span class="cmp-no"><XIcon size={14} /></span>
                    {/if}
                  {:else}
                    <span class="cmp-text">{cmp.lintasan}</span>
                  {/if}
                </td>
                <td class="cmp-cell">
                  {#if typeof cmp.raw === 'boolean'}
                    {#if cmp.raw}
                      <span class="cmp-yes"><Check size={14} /></span>
                    {:else}
                      <span class="cmp-no"><XIcon size={14} /></span>
                    {/if}
                  {:else}
                    <span class="cmp-text">{cmp.raw}</span>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    </section>

    <!-- How it works -->
    <section class="how">
      <div class="section-header">
        <span class="section-badge">Workflow</span>
        <h2 class="section-title">How it works</h2>
        <p class="section-sub">Tiga langkah sederhana untuk mulai production.</p>
      </div>
      <div class="how-grid">
        {#each steps as s, i}
          <article class="how-card" style="animation: fadeInUp 0.5s ease-out {0.1 * i}s both;">
            <span class="step-num">{s.step}</span>
            <h3>{s.title}</h3>
            <p>{s.desc}</p>
          </article>
        {/each}
      </div>
    </section>

    <!-- Testimonial -->
    <section class="testimonials">
      <div class="testi-inner">
        <div class="testi-icon"><Star size={16} /></div>
        <blockquote class="testi-quote">
          "Lintasan changed how we deploy AI features. One endpoint, zero headaches.
          The caching alone cut our API costs by 40%."
        </blockquote>
        <div class="testi-attribution">
          <div class="testi-avatar">ML</div>
          <div>
            <div class="testi-name">ML Engineer</div>
            <div class="testi-company">SaaS Platform</div>
          </div>
        </div>
      </div>
    </section>

    <!-- Final CTA -->
    <section class="final-cta">
      <div class="final-cta-inner">
        <h2>Ready to simplify your AI stack?</h2>
        <p>Start routing smarter in under 5 minutes.</p>
        <a href={ctaHref} class="btn-hero">
          {ctaLabel}
          <ArrowRight size={16} />
        </a>
      </div>
    </section>
  </main>

  <footer class="footer">
    <div class="footer-brand">
      <strong>Lintasan</strong>
      <span>AI Gateway Management</span>
    </div>
    <div class="footer-links">
      <a href="/login">Sign In</a>
      <a href="/dashboard/docs">Documentation</a>
      <a href="https://github.com/sans-haji/lintasan" target="_blank" rel="noopener noreferrer">GitHub</a>
    </div>
  </footer>
</div>

<style>
  .landing {
    position: relative;
    min-height: 100vh;
    background: #ffffff;
    color: #1e293b;
    font-family: 'Inter', system-ui, -apple-system, sans-serif;
    opacity: 0;
    transition: opacity 0.3s ease;
    overflow-x: hidden;
  }
  .landing.mounted { opacity: 1; }

  /* Decorative elements */
  .deco-glow {
    position: fixed;
    border-radius: 50%;
    pointer-events: none;
    filter: blur(120px);
    opacity: 0.15;
    z-index: 0;
  }
  .deco-glow-1 {
    width: 600px; height: 600px;
    top: -200px; right: -200px;
    background: #4f46e5;
    animation: floatGlow 8s ease-in-out infinite;
  }
  .deco-glow-2 {
    width: 400px; height: 400px;
    bottom: 20%; left: -100px;
    background: #7c3aed;
    animation: floatGlow 10s ease-in-out infinite reverse;
  }
  .deco-dots {
    position: absolute;
    pointer-events: none;
    opacity: 0.06;
    z-index: 0;
  }
  .deco-dots-1 {
    top: 15%; right: 8%;
    width: 120px; height: 120px;
    background-image: radial-gradient(circle, #1e293b 1.5px, transparent 1.5px);
    background-size: 16px 16px;
  }
  .deco-dots-2 {
    bottom: 30%; left: 5%;
    width: 80px; height: 80px;
    background-image: radial-gradient(circle, #4f46e5 1.5px, transparent 1.5px);
    background-size: 14px 14px;
  }

  .topbar {
    position: relative;
    z-index: 10;
    display: flex;
    align-items: center;
    justify-content: space-between;
    max-width: 1200px;
    margin: 0 auto;
    padding: 20px 32px;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    text-decoration: none;
  }
  .brand-mark {
    width: 36px; height: 36px;
    border-radius: 10px;
    background: linear-gradient(135deg, #4f46e5, #7c3aed);
    color: #fff;
    display: grid;
    place-items: center;
    font-weight: 700;
    font-size: 15px;
  }
  .brand-name {
    font-size: 18px;
    font-weight: 700;
    color: #1e293b;
    letter-spacing: -0.3px;
  }

  .nav-links { display: flex; align-items: center; gap: 16px; }
  .nav-link {
    text-decoration: none;
    color: #64748b;
    font-size: 14px;
    font-weight: 500;
    padding: 8px 4px;
  }
  .nav-link:hover { color: #1e293b; }

  .btn-cta {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 9px 18px;
    background: #4f46e5;
    color: #fff;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 600;
    text-decoration: none;
    transition: background 0.15s;
  }
  .btn-cta:hover { background: #4338ca; }

  /* Hero */
  .hero {
    position: relative;
    z-index: 5;
    padding: 60px 32px 20px;
  }
  .hero-bg {
    position: absolute;
    inset: 0;
    overflow: hidden;
    pointer-events: none;
  }
  .hero-bg-glow {
    position: absolute;
    top: -50%;
    left: 50%;
    transform: translateX(-50%);
    width: 800px;
    height: 600px;
    background: radial-gradient(ellipse at center, rgba(79,70,229,0.06) 0%, transparent 70%);
    animation: breathe 4s ease-in-out infinite;
  }
  .hero-inner {
    position: relative;
    max-width: 720px;
    margin: 0 auto;
    text-align: center;
    z-index: 2;
  }
  .hero-kicker-wrap {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 14px;
    background: #eef2ff;
    border: 1px solid #c7d2fe;
    border-radius: 999px;
    margin-bottom: 20px;
  }
  .hero-kicker {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 12px;
    font-weight: 600;
    color: #4f46e5;
    letter-spacing: 0.02em;
  }
  .hero-title {
    margin: 0 0 16px;
    font-size: clamp(44px, 7vw, 68px);
    line-height: 1.08;
    letter-spacing: -0.04em;
    color: #0f172a;
    font-weight: 800;
  }
  .hero-title-accent {
    background: linear-gradient(135deg, #4f46e5, #7c3aed);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
  }
  .hero-sub {
    margin: 0 auto 32px;
    max-width: 520px;
    font-size: 17px;
    line-height: 1.65;
    color: #64748b;
  }
  .hero-actions {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    flex-wrap: wrap;
  }
  .btn-hero {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 14px 28px;
    background: linear-gradient(135deg, #4f46e5, #4338ca);
    color: #fff;
    border-radius: 12px;
    font-size: 15px;
    font-weight: 600;
    text-decoration: none;
    transition: transform 0.15s, box-shadow 0.15s;
    box-shadow: 0 8px 24px rgba(79, 70, 229, 0.25);
  }
  .btn-hero:hover { transform: translateY(-2px); box-shadow: 0 12px 32px rgba(79, 70, 229, 0.35); }
  .btn-hero-outline {
    display: inline-flex;
    align-items: center;
    gap: 7px;
    padding: 13px 24px;
    background: transparent;
    color: #475569;
    border: 1px solid #e2e8f0;
    border-radius: 12px;
    font-size: 14px;
    font-weight: 600;
    text-decoration: none;
    transition: all 0.15s;
  }
  .btn-hero-outline:hover {
    border-color: #c7d2fe;
    color: #4f46e5;
    background: #f8fafc;
  }
  .hero-badge-row {
    display: flex;
    justify-content: center;
    gap: 8px;
    margin-top: 24px;
    flex-wrap: wrap;
  }
  .hero-badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 4px 10px;
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    border-radius: 999px;
    font-size: 11px;
    color: #94a3b8;
    font-weight: 500;
  }

  /* Metrics strip */
  .metrics-strip {
    position: relative;
    z-index: 5;
    background: linear-gradient(90deg, #f8fafc 0%, #eef2ff 50%, #f8fafc 100%);
    border-top: 1px solid #e2e8f0;
    border-bottom: 1px solid #e2e8f0;
    padding: 28px 32px;
  }
  .metrics-inner {
    max-width: 960px;
    margin: 0 auto;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
  }
  .metric-item { text-align: center; flex: 1; }
  .metric-val {
    font-size: 28px;
    font-weight: 800;
    letter-spacing: -0.5px;
    background: linear-gradient(135deg, #4f46e5, #7c3aed);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
    line-height: 1;
    margin-bottom: 4px;
    font-variant-numeric: tabular-nums;
  }
  .metric-label {
    font-size: 12px;
    font-weight: 600;
    color: #94a3b8;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .metric-divider {
    width: 1px; height: 40px;
    background: #e2e8f0;
    flex-shrink: 0;
  }

  .section-header {
    text-align: center;
    margin-bottom: 48px;
  }
  .section-badge {
    display: inline-block;
    padding: 4px 12px;
    background: #eef2ff;
    color: #4f46e5;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    margin-bottom: 12px;
  }
  .section-title {
    margin: 0 0 8px;
    font-size: 30px;
    font-weight: 700;
    letter-spacing: -0.03em;
    color: #0f172a;
  }
  .section-sub {
    margin: 0;
    font-size: 16px;
    color: #64748b;
  }

  .features {
    position: relative;
    z-index: 5;
    max-width: 1100px;
    margin: 0 auto;
    padding: 72px 32px;
  }
  .feature-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: 20px;
  }
  .feature-card {
    background: #ffffff;
    border: 1px solid #e2e8f0;
    border-radius: 14px;
    padding: 28px 24px;
    transition: border-color 0.2s, box-shadow 0.2s, transform 0.2s;
  }
  .feature-card:hover {
    border-color: #c7d2fe;
    box-shadow: 0 8px 24px rgba(79, 70, 229, 0.08);
    transform: translateY(-2px);
  }
  .feature-icon {
    width: 44px; height: 44px;
    border-radius: 12px;
    background: linear-gradient(135deg, #eef2ff, #f5f3ff);
    color: #4f46e5;
    display: grid;
    place-items: center;
    margin-bottom: 16px;
  }
  .feature-card h3 {
    margin: 0 0 8px;
    font-size: 16px;
    font-weight: 600;
    color: #1e293b;
  }
  .feature-card p {
    margin: 0;
    font-size: 14px;
    line-height: 1.6;
    color: #64748b;
  }

  /* Comparison table */
  .compare {
    position: relative;
    z-index: 5;
    max-width: 840px;
    margin: 0 auto;
    padding: 72px 32px;
    text-align: center;
  }
  .compare-table-wrap {
    overflow-x: auto;
    text-align: left;
  }
  .compare-table {
    width: 100%;
    border-collapse: collapse;
    border-radius: 14px;
    overflow: hidden;
    border: 1px solid #e2e8f0;
  }
  .compare-table th {
    padding: 16px 20px;
    font-size: 14px;
    font-weight: 600;
    border-bottom: 1px solid #e2e8f0;
  }
  .th-lintasan {
    background: #eef2ff;
    color: #4f46e5;
  }
  .th-lintasan .th-brand-mark {
    display: inline-flex;
    width: 22px; height: 22px;
    border-radius: 6px;
    background: linear-gradient(135deg, #4f46e5, #7c3aed);
    color: #fff;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    font-weight: 700;
    margin-right: 7px;
    vertical-align: middle;
  }
  .th-raw {
    background: #f8fafc;
    color: #64748b;
  }
  .cmp-aspect {
    padding: 12px 20px;
    font-size: 13px;
    font-weight: 500;
    color: #334155;
    border-bottom: 1px solid #f1f5f9;
    white-space: nowrap;
  }
  .cmp-cell {
    padding: 12px 20px;
    border-bottom: 1px solid #f1f5f9;
    text-align: center;
    min-width: 130px;
  }
  .cmp-yes { color: #059669; }
  .cmp-no { color: #dc2626; opacity: 0.5; }
  .cmp-text { font-size: 13px; color: #64748b; }

  /* How it works */
  .how {
    position: relative;
    z-index: 5;
    max-width: 1100px;
    margin: 0 auto;
    padding: 72px 32px;
    text-align: center;
  }
  .how-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: 20px;
  }
  .how-card {
    background: #ffffff;
    border: 1px solid #e2e8f0;
    border-radius: 14px;
    padding: 32px 24px;
    text-align: left;
    transition: transform 0.2s, box-shadow 0.2s;
  }
  .how-card:hover {
    transform: translateY(-2px);
    box-shadow: 0 8px 24px rgba(79, 70, 229, 0.08);
  }
  .step-num {
    display: inline-block;
    font-size: 12px;
    font-weight: 700;
    letter-spacing: 0.04em;
    color: #4f46e5;
    background: linear-gradient(135deg, #eef2ff, #f5f3ff);
    padding: 4px 10px;
    border-radius: 6px;
    margin-bottom: 16px;
  }
  .how-card h3 {
    margin: 0 0 8px;
    font-size: 16px;
    font-weight: 600;
    color: #1e293b;
  }
  .how-card p {
    margin: 0;
    font-size: 14px;
    line-height: 1.6;
    color: #64748b;
  }

  /* Testimonials */
  .testimonials {
    position: relative;
    z-index: 5;
    max-width: 600px;
    margin: 0 auto;
    padding: 48px 32px 72px;
    text-align: center;
  }
  .testi-inner {
    background: linear-gradient(135deg, #eef2ff 0%, #f5f3ff 100%);
    border: 1px solid #c7d2fe;
    border-radius: 16px;
    padding: 40px 36px;
  }
  .testi-icon {
    color: #f59e0b;
    margin-bottom: 16px;
  }
  .testi-quote {
    margin: 0 0 24px;
    font-size: 17px;
    line-height: 1.7;
    color: #1e293b;
    font-weight: 500;
    font-style: italic;
  }
  .testi-attribution {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
  }
  .testi-avatar {
    width: 38px; height: 38px;
    border-radius: 50%;
    background: linear-gradient(135deg, #4f46e5, #7c3aed);
    color: #fff;
    display: grid;
    place-items: center;
    font-size: 13px;
    font-weight: 700;
  }
  .testi-name {
    font-size: 13px;
    font-weight: 600;
    color: #1e293b;
    text-align: left;
  }
  .testi-company {
    font-size: 12px;
    color: #64748b;
    text-align: left;
  }

  /* Final CTA */
  .final-cta {
    position: relative;
    z-index: 5;
    max-width: 600px;
    margin: 0 auto;
    padding: 48px 32px 72px;
    text-align: center;
  }
  .final-cta-inner {
    background: linear-gradient(135deg, #0f172a 0%, #1e293b 100%);
    border: 1px solid #334155;
    border-radius: 20px;
    padding: 56px 40px;
  }
  .final-cta-inner h2 {
    margin: 0 0 8px;
    font-size: 28px;
    font-weight: 700;
    color: #f8fafc;
    letter-spacing: -0.03em;
  }
  .final-cta-inner p {
    margin: 0 0 28px;
    font-size: 16px;
    color: #94a3b8;
  }
  .final-cta-inner .btn-hero {
    box-shadow: 0 8px 24px rgba(79, 70, 229, 0.35);
  }

  /* Footer */
  .footer {
    position: relative;
    z-index: 5;
    max-width: 1100px;
    margin: 0 auto;
    padding: 32px;
    border-top: 1px solid #e2e8f0;
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 16px;
  }
  .footer-brand strong {
    display: block;
    font-size: 15px;
    color: #1e293b;
  }
  .footer-brand span {
    font-size: 13px;
    color: #94a3b8;
  }
  .footer-links {
    display: flex;
    gap: 20px;
  }
  .footer-links a {
    text-decoration: none;
    font-size: 13px;
    color: #64748b;
  }
  .footer-links a:hover { color: #1e293b; }

  @keyframes spin { to { transform: rotate(360deg); } }
  @keyframes floatGlow {
    0%, 100% { transform: translate(0, 0); }
    33% { transform: translate(20px, -20px); }
    66% { transform: translate(-10px, 10px); }
  }
  @keyframes breathe {
    0%, 100% { opacity: 0.4; }
    50% { opacity: 0.8; }
  }

  @media (max-width: 640px) {
    .topbar { padding: 16px 20px; }
    .hero { padding: 40px 20px 20px; }
    .hero-title { font-size: 36px; }
    .metrics-strip { padding: 20px; }
    .metrics-inner { flex-wrap: wrap; gap: 20px; }
    .metric-val { font-size: 22px; }
    .metric-divider:last-of-type { display: none; }
    .features, .how { padding: 48px 20px; }
    .feature-grid { grid-template-columns: 1fr; }
    .how-grid { grid-template-columns: 1fr; }
    .compare { padding: 48px 20px; }
    .compare-table { font-size: 12px; }
    .testimonials { padding: 32px 20px 48px; }
    .testi-inner { padding: 28px 20px; }
    .testi-quote { font-size: 15px; }
    .final-cta { padding: 32px 20px 48px; }
    .final-cta-inner { padding: 36px 24px; }
    .final-cta-inner h2 { font-size: 22px; }
    .footer { flex-direction: column; align-items: flex-start; }
    .deco-glow-1 { width: 300px; height: 300px; }
    .deco-dots-1 { display: none; }
  }
</style>
