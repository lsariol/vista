'use strict';
// Vista's dashboard. Every value from a source is put on the page as text
// (textContent, attributes), never as HTML: log lines are untrusted.
(() => {
  // ---------- DOM helpers ----------
  const $ = (sel, root = document) => root.querySelector(sel);
  const SVGNS = 'http://www.w3.org/2000/svg';

  function h(tag, attrs, ...kids) {
    const el = document.createElement(tag);
    setAttrs(el, attrs);
    add(el, kids);
    return el;
  }
  function s(tag, attrs, ...kids) {
    const el = document.createElementNS(SVGNS, tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (v != null) el.setAttribute(k, String(v));
    }
    add(el, kids);
    return el;
  }
  // Line icons, drawn on a 24-unit grid with the text color.
  const ICONS = {
    home: 'M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z',
    box: 'M21 8 12 3 3 8v8l9 5 9-5zM3 8l9 5 9-5M12 13v8',
    server: 'M4 4h16v6H4zM4 14h16v6H4zM8 7h.01M8 17h.01M12 7h4M12 17h4',
    pulse: 'M3 12h4l3-8 4 16 3-8h4',
    logs: 'M6 3h9l4 4v14H6zM14 3v5h5M9 12h7M9 16h7',
    alert: 'M12 3 2 20h20zM12 10v4M12 17h.01',
    bell: 'M6 9a6 6 0 1 1 12 0c0 6 3 8 3 8H3s3-2 3-8M10.3 21a2 2 0 0 0 3.4 0',
    layers: 'M12 3 2 8l10 5 10-5zM2 13l10 5 10-5M2 17.5l10 5 10-5',
    cpu: 'M7 7h10v10H7zM10 10h4v4h-4zM9 2v3M15 2v3M9 19v3M15 19v3M2 9h3M2 15h3M19 9h3M19 15h3',
    memory: 'M3 7h18v9H3zM7 10v3M11 10v3M15 10v3M6 16v3M10 16v3M14 16v3M18 16v3',
    disk: 'M4 6c0-1.7 3.6-3 8-3s8 1.3 8 3-3.6 3-8 3-8-1.3-8-3zM4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
    network: 'M7 4v16M3 16l4 4 4-4M17 20V4M13 8l4-4 4 4',
    chart: 'M3 3v18h18M7 15l4-5 3 3 5-7',
    deploy: 'M12 16V4M7 9l5-5 5 5M4 20h16',
    errors: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 8v5M12 16h.01',
    pie: 'M12 3a9 9 0 1 0 9 9h-9zM15 2.5A9 9 0 0 1 21.5 9H15z',
    gauge: 'M12 14l4-4M3.5 18a9 9 0 1 1 17 0',
    globe: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM3 12h18M12 3c3 3.5 3 14.5 0 18M12 3c-3 3.5-3 14.5 0 18',
    target: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
    bars: 'M4 20V10M10 20V4M16 20v-7M22 20H2',
    search: 'M11 4a7 7 0 1 0 0 14 7 7 0 0 0 0-14zM20 20l-4-4',
  };
  function icon(name, size = 18) {
    return s('svg', { class: 'icon', width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor',
      'stroke-width': 1.8, 'stroke-linecap': 'round', 'stroke-linejoin': 'round', 'aria-hidden': 'true' }, s('path', { d: ICONS[name] || ICONS.layers }));
  }
  // The panels name their icons with a character; this is the drawing for each.
  const GLYPH = { '◇': 'box', '⚠': 'errors', '♧': 'bell', '〽': 'chart', '⇪': 'deploy', 'ϟ': 'server', '○': 'target', '◉': 'layers', '▤': 'logs',
    '⌁': 'pulse', '⌖': 'target', '▥': 'bars', '⇅': 'network', '◫': 'disk', '⛁': 'disk', '◎': 'globe', '▣': 'cpu' };
  const iconFor = (g) => h('span', { class: 'ico' }, icon(GLYPH[g] || g, 16));

  function setAttrs(el, attrs) {
    for (const [k, v] of Object.entries(attrs || {})) {
      if (v == null || v === false) continue;
      if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
      else if (k === 'class') el.className = v;
      else if (k === 'href') el.setAttribute('href', safeHref(v));
      else if (k === 'style') Object.assign(el.style, v);
      else if (v === true) el.setAttribute(k, '');
      else el.setAttribute(k, String(v));
    }
  }
  function add(el, kids) {
    for (const k of kids.flat(Infinity)) {
      if (k == null || k === false) continue;
      el.append(k instanceof Node ? k : String(k));
    }
  }
  // Only links inside Vista are made from data.
  const safeHref = (v) => (String(v).startsWith('#/') ? String(v) : '#/');

  // ---------- Formatting ----------
  const isNum = (v) => typeof v === 'number' && Number.isFinite(v);
  const fmtPct = (v, d = 0) => (isNum(v) ? `${v.toFixed(d)}%` : '—');
  const fmtUptime = (u) => {
    if (!u || !isNum(u.pct)) return '—';
    return `${u.pct >= 99.95 ? u.pct.toFixed(2) : u.pct.toFixed(1)}%`;
  };
  function fmtBytes(n) {
    if (!isNum(n)) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
    return `${n.toFixed(n >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
  }
  function fmtSeconds(sec) {
    if (!isNum(sec)) return '—';
    if (sec < 90) return `${Math.round(sec)} s`;
    if (sec < 5400) return `${Math.round(sec / 60)} min`;
    return `${(sec / 3600).toFixed(1)} h`;
  }
  const fmtMinutes = (m) => (m < 90 ? `${m} min` : `${(m / 60).toFixed(1)} h`);
  // Network rates are shown in bits per second, as links are sold.
  function fmtBits(bytesPerS) {
    if (!isNum(bytesPerS)) return '—';
    const b = bytesPerS * 8;
    if (b >= 1e9) return `${(b / 1e9).toFixed(1)} Gbps`;
    if (b >= 1e6) return `${(b / 1e6).toFixed(b >= 1e8 ? 0 : 1)} Mbps`;
    if (b >= 1e3) return `${(b / 1e3).toFixed(b >= 1e5 ? 0 : 1)} kbps`;
    return `${Math.round(b)} bps`;
  }
  const fmtRate = (bytesPerS) => (isNum(bytesPerS) ? `${fmtBytes(bytesPerS)}/s` : '—');
  const fmtCores = (c) => (isNum(c) ? `${(c * 100).toFixed(c >= 0.1 ? 0 : 1)}%` : '—');
  function fmtDuration(sec) {
    if (!isNum(sec)) return '—';
    const d = Math.floor(sec / 86400), hr = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
    if (d) return `${plural(d, 'day', 'days')}, ${plural(hr, 'hour', 'hours')}`;
    if (hr) return `${plural(hr, 'hour', 'hours')}, ${m} min`;
    return `${m} min`;
  }
  const fmtCount = (n) => (isNum(n) ? n.toLocaleString() : '—');
  const toDate = (iso) => (iso ? new Date(iso) : null);
  function fmtTime(iso) {
    const d = toDate(iso);
    if (!d) return '—';
    const sameDay = d.toDateString() === new Date().toDateString();
    const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    return sameDay ? time : `${d.toLocaleDateString([], { day: 'numeric', month: 'short' })}, ${time}`;
  }
  function fmtFull(iso) {
    const d = toDate(iso);
    return d ? d.toLocaleString([], { dateStyle: 'medium', timeStyle: 'medium' }) : '—';
  }
  function ago(iso) {
    const d = toDate(iso);
    if (!d) return '—';
    const sec = (Date.now() - d.getTime()) / 1000;
    if (sec < 45) return 'just now';
    if (sec < 3600) return `${Math.round(sec / 60)} min ago`;
    if (sec < 86400 * 2) return `${Math.round(sec / 3600)} h ago`;
    return `${Math.round(sec / 86400)} days ago`;
  }
  const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

  const LABEL = {
    healthy: 'Healthy', updating: 'Updating', degraded: 'Degraded', down: 'Down', unknown: 'Unknown',
    ok: 'Answering', stale: 'Stale', unavailable: 'Unavailable', not_connected: 'Not connected',
    warn: 'Warning', info: 'Note',
  };
  function badge(state, text) {
    return h('span', { class: 'state' }, h('i', { class: `dot ${state}` }), h('span', { class: `t-${state}` }, text || LABEL[state] || state));
  }
  function projectBadge(p) {
    if (p.state === 'unknown' && p.last_known) {
      return h('span', { title: 'Sonar isn\'t answering; this was its last report.' },
        badge('unknown'), h('span', { class: 'muted small' }, ` (last: ${LABEL[p.last_known] || p.last_known})`));
    }
    return badge(p.state);
  }

  // ---------- API ----------
  async function api(path) {
    let res;
    try {
      res = await fetch(path, { headers: { Accept: 'application/json' }, credentials: 'same-origin' });
    } catch {
      throw new Error('Vista can\'t be reached. If your sign-in has expired, reload the page.');
    }
    let body = null;
    try { body = await res.json(); } catch { /* not JSON */ }
    if (!res.ok) throw new Error(body && body.error ? body.error : `Vista answered ${res.status}.`);
    if (body === null) throw new Error('Vista sent an unreadable answer. If your sign-in has expired, reload the page.');
    return body;
  }

  // ---------- State ----------
  const store = {
    get(k, def) { try { return localStorage.getItem(k) ?? def; } catch { return def; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch { /* private window */ } },
  };
  const state = {
    range: ['1h', '24h', '7d', '30d'].includes(store.get('vista.range')) ? store.get('vista.range') : '1h',
    overview: null,
    projectFilter: '',
    projectStatus: 'all',
    containerFilter: '',
    containerStatus: 'all',
    topBy: 'cpu',
    token: 0,
  };

  // ---------- Pages and routing ----------
  const PAGES = [
    { id: 'overview', label: 'Overview', icon: 'home', render: renderOverview, auto: true, sub: 'What needs attention on Spark and its projects' },
    { id: 'projects', label: 'Projects', icon: 'box', render: renderProjects, auto: true, sub: 'Every project Sonar watches, with its version and availability' },
    { id: 'spark', label: 'Spark', icon: 'server', render: renderSpark, auto: true, sub: 'The server itself: its resources and the infrastructure every project relies on' },
    { id: 'activity', label: 'Activity', icon: 'pulse', render: renderActivity, auto: true, sub: 'Deploys, health changes, errors and log gaps on one timeline' },
    { id: 'logs', label: 'Logs', icon: 'logs', render: renderLogs, auto: false, sub: 'Search what every container wrote, kept by Blackbox' },
    { id: 'incidents', label: 'Incidents', icon: 'bell', render: renderIncidents, auto: true, sub: 'Availability and the days projects were down or degraded' },
    { id: 'sources', label: 'Sources', icon: 'layers', render: renderSources, auto: true, sub: 'Where Vista\'s information comes from, and how fresh it is' },
  ];

  function parseHash() {
    const raw = location.hash.replace(/^#\/?/, '');
    const [path, qs = ''] = raw.split('?');
    const parts = path.split('/').filter(Boolean).map((p) => { try { return decodeURIComponent(p); } catch { return p; } });
    const page = PAGES.find((p) => p.id === parts[0]) || PAGES[0];
    return { page, arg: parts[1] || '', params: new URLSearchParams(qs) };
  }
  const hashFor = (page, params) => {
    const qs = new URLSearchParams(Object.entries(params || {}).filter(([, v]) => v != null && v !== '')).toString();
    return `#/${page}${qs ? `?${qs}` : ''}`;
  };

  function buildNav() {
    $('#nav').replaceChildren(...PAGES.map((p) => h('a', { href: `#/${p.id}`, 'data-page': p.id },
      h('span', { class: 'nav-ico', 'aria-hidden': 'true' }, icon(p.icon)),
      h('span', { class: 'nav-label' }, p.label),
      p.id === 'overview' ? h('span', { class: 'nav-count', id: 'navCount', hidden: true }) : null)));
  }

  async function route({ quiet = false } = {}) {
    const { page, arg, params } = parseHash();
    const token = ++state.token;
    document.querySelectorAll('#nav a').forEach((a) => {
      if (a.dataset.page === page.id) a.setAttribute('aria-current', 'page');
      else a.removeAttribute('aria-current');
    });
    $('#range').value = ['1h', '24h', '7d', '30d'].includes(params.get('range')) ? params.get('range') : state.range;
    document.title = page.id === 'overview' ? 'Vista' : `${page.label} · Vista`;
    const target = $('#page');
    if (!quiet) target.replaceChildren(h('div', { class: 'loading' }, 'Loading…'));
    let nodes;
    try {
      nodes = await page.render({ params, arg, page });
    } catch (err) {
      nodes = [heading(page), h('div', { class: 'error-box' }, err.message)];
    }
    if (token !== state.token) return; // a newer navigation won
    target.replaceChildren(...[nodes].flat(Infinity).filter(Boolean));
    if (page.id === 'projects' && arg) openProject(arg);
    else if (!quiet) closeDrawer(false);
  }

  function heading(page, extra) {
    return h('div', { class: 'heading' },
      h('div', null, h('h1', null, page.label), h('div', { class: 'subtitle' }, page.sub)),
      extra || null);
  }

  function setUpdated(iso) {
    const el = $('#updated');
    el.textContent = iso ? `Updated ${new Date(iso).toLocaleTimeString()}` : '';
    el.title = iso ? fmtFull(iso) : '';
  }

  async function loadOverview() {
    const o = await api(`/api/v1/overview?range=${encodeURIComponent(state.range)}`);
    state.overview = o;
    setUpdated(o.generated_at);
    const serious = o.attention.filter((a) => a.level === 'down' || a.level === 'degraded').length;
    const count = $('#navCount');
    if (count) {
      count.hidden = serious === 0;
      count.textContent = String(serious);
      count.title = `${plural(serious, 'problem', 'problems')} need attention`;
    }
    return o;
  }

  // ---------- Shared pieces ----------
  function sourcesPill(sources) {
    const bad = sources.filter((x) => x.state === 'unavailable' || x.state === 'stale');
    if (bad.length) {
      return h('a', { href: '#/sources', class: 'pill warn' }, '● ', `${bad.map((x) => x.name).join(', ')} not answering`);
    }
    const off = sources.filter((x) => x.state === 'not_connected').length;
    return h('a', { href: '#/sources', class: 'pill' }, '● ', off ? `Sources answering · ${off} not connected` : 'All sources answering');
  }

  // tile is one of the Overview's stat tiles.
  function tile(title, ico, big, small, extra, link) {
    return h('article', { class: 'card kpi' },
      h('div', { class: 'kpi-top' }, link ? h('a', { href: link }, title) : title),
      h('div', { class: 'kpi-body' }, h('span', { class: 'kpi-ico' }, icon(ico, 30)),
        h('div', { class: 'kpi-values' }, h('div', { class: 'big' }, big), small ? h('div', { class: 'small' }, small) : null)),
      extra || null);
  }

  // sparkArea is a small filled trend, scaled to its own range so movement shows.
  let sparkId = 0;
  function sparkArea(values, color) {
    const W = 120, H = 34;
    const nums = values.filter(isNum);
    let lo = Math.min(...nums), hi = Math.max(...nums);
    const pad = Math.max((hi - lo) * 0.25, Math.abs(hi) * 0.02, 1e-9);
    lo -= pad; hi += pad;
    const pts = values.map((v, i) => (isNum(v) ? [i / Math.max(1, values.length - 1) * W, H - 1 - (v - lo) / (hi - lo) * (H - 3)] : null));
    const svg = s('svg', { class: 'spark-area', viewBox: `0 0 ${W} ${H}`, preserveAspectRatio: 'none', 'aria-hidden': 'true' });
    const id = `sg${++sparkId}`;
    svg.append(s('defs', null, s('linearGradient', { id, x1: 0, y1: 0, x2: 0, y2: 1 },
      s('stop', { offset: '0%', 'stop-color': color, 'stop-opacity': 0.35 }), s('stop', { offset: '100%', 'stop-color': color, 'stop-opacity': 0 }))));
    for (const seg of segments(pts.map((p) => (p ? { x: p[0], y: p[1], v: 1 } : { v: null })))) {
      const line = seg.map((p, i) => `${i ? 'L' : 'M'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
      svg.append(s('path', { d: `${line} L${seg[seg.length - 1].x.toFixed(1)},${H} L${seg[0].x.toFixed(1)},${H} Z`, fill: `url(#${id})` }),
        s('path', { d: line, fill: 'none', stroke: color, 'stroke-width': 1.5, 'vector-effect': 'non-scaling-stroke' }));
    }
    return svg;
  }

  function dayStrip(days, large) {
    if (!days || !days.length) return h('span', { class: 'muted' }, '—');
    return h('div', { class: `days${large ? ' large' : ''}`, role: 'img', 'aria-label': `${days.length} days of history` },
      days.map((d) => h('span', { class: `day ${d.state}`, title: dayTitle(d) })));
  }
  function dayTitle(d) {
    if (d.state === 'no_data') return `${d.date} (UTC): no data`;
    const parts = [`${d.date} (UTC): ${isNum(d.uptime_pct) ? `${d.uptime_pct.toFixed(2)}% up` : 'not enough data'}`];
    if (d.down_minutes) parts.push(`${fmtMinutes(d.down_minutes)} down`);
    if (d.degraded_minutes) parts.push(`${fmtMinutes(d.degraded_minutes)} degraded`);
    if (d.updates) parts.push(plural(d.updates, 'deploy', 'deploys'));
    parts.push(`${Math.round(d.coverage_pct)}% observed`);
    return parts.join(', ');
  }

  // ---------- Charts ----------
  // A time chart drawn at the container's width; nulls are gaps.
  function lineChart(opts) {
    const wrap = h('div', { class: 'chart-wrap' });
    let lastW = 0;
    const ro = new ResizeObserver(() => {
      if (!wrap.isConnected) { ro.disconnect(); return; }
      const w = Math.floor(wrap.clientWidth);
      if (w > 0 && w !== lastW) { lastW = w; drawChart(wrap, w, opts); }
    });
    ro.observe(wrap);
    return wrap;
  }

  function drawChart(wrap, W, { height = 190, from, to, series, markers = [], label, yMax = 100,
    axis = (v) => `${Math.round(v)}%`, tipFmt = (v) => fmtPct(v, 1) }) {
    const H = height, pad = { l: 52, r: 10, t: 10, b: 22 };
    const x0 = pad.l, x1 = W - pad.r, y0 = pad.t, y1 = H - pad.b;
    const all = series.flatMap((se) => se.points.map((p) => p.t));
    const tMin = from ?? Math.min(...all), tMax = to ?? Math.max(...all);
    const span = Math.max(1, tMax - tMin);
    const X = (t) => x0 + (x1 - x0) * (t - tMin) / span;
    const Y = (v) => y1 - (y1 - y0) * Math.max(0, Math.min(v, yMax)) / yMax;
    const svg = s('svg', { class: 'chart', width: W, height: H, viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': label });

    for (const f of [0, 0.25, 0.5, 0.75, 1]) {
      const y = Y(f * yMax);
      svg.append(s('line', { class: 'grid-line', x1: x0, x2: x1, y1: y, y2: y }),
        s('text', { x: x0 - 6, y: y + 3, 'text-anchor': 'end' }, axis(f * yMax)));
    }
    const ticks = W < 500 ? 3 : 6;
    for (let i = 0; i <= ticks; i++) {
      const t = tMin + span * i / ticks;
      const anchor = i === 0 ? 'start' : i === ticks ? 'end' : 'middle';
      svg.append(s('text', { x: X(t), y: H - 5, 'text-anchor': anchor }, tickLabel(t, span)));
    }
    for (const m of markers) {
      if (m.t < tMin || m.t > tMax) continue;
      svg.append(s('line', { x1: X(m.t), x2: X(m.t), y1: y0, y2: y1, stroke: m.color, 'stroke-width': 1.2, 'stroke-dasharray': '3 3', opacity: 0.8 },
        s('title', null, m.label)));
    }
    for (const se of series) {
      for (const seg of segments(se.points)) {
        if (se.band) {
          const top = seg.map((p) => `${X(p.t).toFixed(1)},${Y(p.max).toFixed(1)}`);
          const bottom = seg.slice().reverse().map((p) => `${X(p.t).toFixed(1)},${Y(p.min).toFixed(1)}`);
          svg.append(s('polygon', { points: [...top, ...bottom].join(' '), fill: se.color, opacity: 0.12 }));
        }
        if (seg.length === 1) {
          svg.append(s('circle', { cx: X(seg[0].t), cy: Y(seg[0].v), r: 1.8, fill: se.color }));
        } else {
          const d = seg.map((p, i) => `${i ? 'L' : 'M'}${X(p.t).toFixed(1)},${Y(p.v).toFixed(1)}`).join(' ');
          svg.append(s('path', { d, fill: 'none', stroke: se.color, 'stroke-width': 1.7, 'stroke-linejoin': 'round', 'stroke-linecap': 'round' }));
        }
      }
    }

    // Hover: a crosshair and the values at the nearest bucket.
    const cross = s('line', { y1: y0, y2: y1, stroke: '#9fc6ea', 'stroke-width': 1, opacity: 0 });
    const hit = s('rect', { x: x0, y: y0, width: Math.max(0, x1 - x0), height: Math.max(0, y1 - y0), fill: 'transparent' });
    svg.append(cross, hit);
    const tip = h('div', { class: 'chart-tip', hidden: true });
    const base = series[0] ? series[0].points : [];
    hit.addEventListener('mousemove', (ev) => {
      if (!base.length) return;
      const rect = svg.getBoundingClientRect();
      const t = tMin + (ev.clientX - rect.left - x0) / (x1 - x0) * span;
      let best = 0;
      for (let i = 1; i < base.length; i++) if (Math.abs(base[i].t - t) < Math.abs(base[best].t - t)) best = i;
      const px = X(base[best].t);
      cross.setAttribute('x1', px); cross.setAttribute('x2', px); cross.setAttribute('opacity', 0.6);
      const rows = [h('div', { class: 'muted' }, fmtFull(new Date(base[best].t).toISOString()))];
      for (const se of series) {
        const p = se.points[best];
        let v = p && isNum(p.v) ? tipFmt(p.v) : 'no data';
        if (p && isNum(p.v) && se.band) v += ` (${tipFmt(p.min)}–${tipFmt(p.max)})`;
        rows.push(h('div', null, h('span', { style: { color: se.color } }, '● '), `${se.name}: ${v}`));
      }
      const near = markers.filter((m) => Math.abs(X(m.t) - px) < 6);
      for (const m of near) rows.push(h('div', { style: { color: m.color } }, m.label));
      tip.replaceChildren(...rows);
      tip.hidden = false;
      const left = Math.min(Math.max(px + 12, 0), W - 190);
      tip.style.left = `${left}px`;
      tip.style.top = `${y0}px`;
    });
    hit.addEventListener('mouseleave', () => { tip.hidden = true; cross.setAttribute('opacity', 0); });
    wrap.replaceChildren(svg, tip);
  }

  function niceMax(v) {
    if (!isNum(v) || v <= 0) return 1;
    const p = 10 ** Math.floor(Math.log10(v));
    for (const m of [1, 2, 2.5, 5, 10]) if (m * p >= v) return m * p;
    return 10 * p;
  }

  function segments(points) {
    const out = [];
    let cur = [];
    for (const p of points) {
      if (isNum(p.v)) cur.push(p);
      else if (cur.length) { out.push(cur); cur = []; }
    }
    if (cur.length) out.push(cur);
    return out;
  }

  function tickLabel(t, span) {
    const d = new Date(t);
    if (span <= 36 * 3600e3) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    return d.toLocaleDateString([], { weekday: 'short', day: 'numeric' });
  }

  function hostChart(host, { height, band } = {}) {
    if (!host.points.length) {
      return h('div', { class: 'empty' }, 'No host history: Sonar can\'t be read.');
    }
    const pts = host.points.map((p) => ({ t: Date.parse(p.t), p }));
    const pick = (key) => pts.map(({ t, p }) => ({ t, v: p[key] ? p[key].avg : null, min: p[key] ? p[key].min : null, max: p[key] ? p[key].max : null }));
    const disk = pts.map(({ t, p }) => ({ t, v: isNum(p.disk_used_bytes) && p.disk_total_bytes ? p.disk_used_bytes / p.disk_total_bytes * 100 : null }));
    const markers = host.deploys.map((d) => ({
      t: Date.parse(d.started),
      color: d.outcome === 'ok' ? '#20d58a' : '#fc526b',
      label: `${d.project} deploy ${d.outcome === 'ok' ? '' : `${d.outcome} `}${d.to || ''}`.replace(/\s+/g, ' ').trim(),
    }));
    const hasDisk = disk.some((p) => isNum(p.v));
    return [
      lineChart({
        height: height || 190, band,
        from: host.from ? Date.parse(host.from) : undefined,
        to: host.to ? Date.parse(host.to) : undefined,
        label: 'Host CPU, memory and disk use over time',
        series: [
          { name: 'CPU', color: COLORS.cpu, points: pick('cpu_pct'), band },
          { name: 'Memory', color: COLORS.mem, points: pick('mem_used_pct'), band },
          hasDisk ? { name: 'Disk', color: COLORS.disk, points: disk } : null,
        ].filter(Boolean),
        markers,
      }),
      h('div', { class: 'legend' },
        h('span', null, h('i', { style: { background: COLORS.cpu } }), 'CPU'),
        h('span', null, h('i', { style: { background: COLORS.mem } }), 'Memory'),
        hasDisk ? h('span', null, h('i', { style: { background: COLORS.disk } }), 'Disk') : null,
        h('span', null, h('i', { class: 'deploy' }), 'Deploy'),
        h('span', null, h('i', { class: 'deploy-bad' }), 'Failed deploy')),
    ];
  }

  const COLORS = { cpu: '#30a7ff', mem: '#ae75fc', disk: '#20d58a', rx: '#30a7ff', tx: '#20d58a', err: '#fc526b', warn: '#ffbe45' };

  // mainInterface is the host's network link: the first with current rates.
  function mainInterface(host) {
    const now = host.now && host.now.network ? host.now.network : [];
    const live = now.find((n) => isNum(n.rx_bytes_per_s));
    const name = live ? live.interface : (host.network[0] && host.network[0].interface);
    return { name, now: live || null, series: host.network.find((n) => n.interface === name) || null };
  }

  function networkChart(host, { height = 150 } = {}) {
    const { name, series } = mainInterface(host);
    if (!series || !series.points.length) return h('div', { class: 'empty' }, 'No network history: Sonar doesn\'t serve it yet.');
    const rate = (p, key) => (p.minutes ? p[key] / (p.minutes * 60) : null);
    const rx = series.points.map((p) => ({ t: Date.parse(p.t), v: rate(p, 'rx_bytes') }));
    const tx = series.points.map((p) => ({ t: Date.parse(p.t), v: rate(p, 'tx_bytes') }));
    const top = niceMax(Math.max(...rx.map((p) => p.v || 0), ...tx.map((p) => p.v || 0)) * 1.1);
    return [
      lineChart({ height, yMax: top, axis: fmtBits, tipFmt: fmtBits, label: `Network traffic on ${name}`,
        from: host.from ? Date.parse(host.from) : undefined, to: host.to ? Date.parse(host.to) : undefined,
        series: [{ name: 'Download', color: COLORS.rx, points: rx }, { name: 'Upload', color: COLORS.tx, points: tx }] }),
      h('div', { class: 'legend' },
        h('span', null, h('i', { style: { background: COLORS.rx } }), 'Download'),
        h('span', null, h('i', { style: { background: COLORS.tx } }), 'Upload'),
        h('span', { class: 'muted' }, `Average per ${fmtSeconds(host.bucket_seconds)} on ${name}`)),
    ];
  }

  function networkCard(host) {
    const { name, now } = mainInterface(host);
    return h('article', { class: 'card panel' },
      panelHead('⇅', 'Network I/O', h('span', { class: 'muted' }, rangeLabel(host.range))),
      h('div', { class: 'net-now' },
        h('div', null, h('div', { class: 'big blue' }, '↓ ', now ? fmtBits(now.rx_bytes_per_s) : '—'), h('div', { class: 'small' }, 'Download')),
        h('div', null, h('div', { class: 'big green' }, '↑ ', now ? fmtBits(now.tx_bytes_per_s) : '—'), h('div', { class: 'small' }, 'Upload'))),
      networkChart(host),
      name ? null : h('p', { class: 'note' }, 'Sonar hasn\'t read the host\'s network yet.'));
  }

  const panelHead = (ico, title, extra) => h('div', { class: 'panel-head' },
    h('div', { class: 'panel-title' }, iconFor(ico), title), extra || null);

  // bar is a labelled meter; pct is of the whole track.
  function meter(label, pct, value, color) {
    const cls = !color ? (pct > 90 ? 'bad' : pct > 75 ? 'warn' : '') : '';
    return h('div', { class: 'bar-row' },
      h('span', { class: 'bar-label', title: label }, label),
      h('span', { class: 'bar-pct' }, isNum(pct) ? fmtPct(pct, pct < 10 && pct > 0 ? 1 : 0) : '—'),
      h('div', { class: 'bar-track' }, h('div', { class: `bar-fill ${cls}`, style: { width: `${Math.min(100, Math.max(0, pct || 0))}%`, background: color || null } })),
      h('span', { class: 'bar-value' }, value));
  }

  function serverCard(host, containers) {
    const n = host.now;
    const running = containers ? containers.filter((c) => c.state === 'running').length : null;
    return h('article', { class: 'card panel' },
      panelHead('▣', 'Server information'),
      n ? [
        kv('Uptime', fmtDuration(n.uptime_seconds)),
        kv('Started', n.booted_at ? fmtFull(n.booted_at) : '—'),
        kv('CPU', n.core_pcts && n.core_pcts.length ? `${plural(n.core_pcts.length, 'core', 'cores')}, ${fmtPct(n.cpu_pct, 1)} in use` : fmtPct(n.cpu_pct, 1)),
        kv('Memory', `${fmtBytes(n.memory_total)} (${fmtBytes(n.memory_used)} used)`),
        kv('Swap', n.swap_total ? `${fmtBytes(n.swap_total)} (${fmtBytes(n.swap_used)} used)` : 'None'),
        kv('Load average', `${n.load1.toFixed(2)} · ${n.load5.toFixed(2)} · ${n.load15.toFixed(2)}`),
        kv('Disk', n.disk ? `${fmtBytes(n.disk.total)} (${fmtBytes(n.disk.available)} free)` : '—'),
        kv('Containers', containers ? `${containers.length} (${running} running, ${containers.length - running} not)` : '—'),
        kv('Read by Sonar', `${fmtTime(n.read_at)} (${ago(n.read_at)})`),
      ] : h('div', { class: 'empty' }, 'Sonar hasn\'t read the host yet, or can\'t be read.'));
  }

  function diskCard(host, disk, logs) {
    const d = host.now && host.now.disk;
    const rows = [];
    if (d) rows.push(meter('Filesystem', d.used_pct, `${fmtBytes(d.used)} / ${fmtBytes(d.total)}`));
    if (d && disk) {
      const t = disk.totals;
      const share = (b) => b / d.total * 100;
      if (isNum(t.data_bytes)) rows.push(meter('Data folders', share(t.data_bytes), fmtBytes(t.data_bytes), '#ffa53d'));
      rows.push(meter('Images', share(t.image_bytes), fmtBytes(t.image_bytes), '#30a7ff'));
      rows.push(meter('Volumes', share(t.volume_bytes), fmtBytes(t.volume_bytes), '#ae75fc'));
      rows.push(meter('Build cache', share(t.build_cache_bytes), fmtBytes(t.build_cache_bytes), '#8397b0'));
    }
    if (logs && logs.max_bytes) rows.push(meter('Logs (cap)', logs.stored_bytes / logs.max_bytes * 100, `${fmtBytes(logs.stored_bytes)} / ${fmtBytes(logs.max_bytes)}`, '#20d58a'));
    return h('article', { class: 'card panel' },
      panelHead('◫', 'Disk usage', disk ? h('span', { class: 'muted', title: fmtFull(disk.measured_at) }, `measured ${ago(disk.measured_at)}`) : null),
      rows.length ? h('div', { class: 'bars' }, rows) : h('div', { class: 'empty' }, 'Sonar hasn\'t measured the disk yet, or can\'t be read.'),
      h('p', { class: 'note' }, 'Shares are of the whole disk. Logs are measured against Blackbox\'s own cap.'));
  }

  function databasesCard(db) {
    return h('article', { class: 'card panel' },
      panelHead('⛁', 'Databases', db ? h('span', { class: 'muted' }, `sparkdb, read ${ago(db.read_at)}`) : null),
      db ? [
        meter('Connections', db.connections / db.max_connections * 100, `${db.connections} / ${db.max_connections}`),
        h('div', { class: 'scroll-x' }, h('table', { class: 'table' },
          h('thead', null, h('tr', null, h('th', null, 'Database'), h('th', null, 'Project'), h('th', { class: 'num' }, 'Size'), h('th', { class: 'num' }, 'Connections'))),
          h('tbody', null, db.databases.slice().sort((a, b) => b.size_bytes - a.size_bytes).map((d) => h('tr', null,
            h('td', { class: 'mono' }, d.name), h('td', null, d.project || '—'), h('td', { class: 'num' }, fmtBytes(d.size_bytes)),
            h('td', { class: 'num' }, `${d.connections}${d.active ? ` (${d.active} active)` : ''}`)))))),
      ] : h('div', { class: 'empty' }, 'Sonar hasn\'t read sparkdb yet, or can\'t be read.'));
  }

  function diskByProjectCard(disk) {
    if (!disk) return h('article', { class: 'card panel' }, panelHead('◫', 'Disk use by project'), h('div', { class: 'empty' }, 'Sonar hasn\'t measured it yet, or can\'t be read.'));
    const rows = disk.projects.map((p) => ({ ...p, total: (p.data_bytes || 0) + p.image_bytes + p.container_bytes + p.volume_bytes }))
      .sort((a, b) => b.total - a.total);
    return h('article', { class: 'card panel' }, panelHead('◫', 'Disk use by project'),
      h('div', { class: 'scroll-x' }, h('table', { class: 'table' },
        h('thead', null, h('tr', null, h('th', null, 'Project'), h('th', { class: 'num' }, 'Data folder'), h('th', { class: 'num' }, 'Images'),
          h('th', { class: 'num' }, 'Volumes'), h('th', { class: 'num' }, 'In all'))),
        h('tbody', null, rows.map((p) => h('tr', { class: 'clickable', onclick: () => openProject(p.project) },
          h('td', { class: 'name-cell' }, p.project),
          h('td', { class: 'num' }, isNum(p.data_bytes) ? fmtBytes(p.data_bytes) : '—', p.data_unreadable ? h('div', { class: 'cov' }, `${p.data_unreadable} unreadable`) : null),
          h('td', { class: 'num' }, fmtBytes(p.image_bytes)), h('td', { class: 'num' }, fmtBytes(p.volume_bytes)), h('td', { class: 'num' }, fmtBytes(p.total))))))),
      h('p', { class: 'note' }, 'Images count only layers no other project shares.'));
  }

  function trafficCard(traffic, range) {
    return h('article', { class: 'card panel' },
      panelHead('◎', 'Traffic per site', h('span', { class: 'muted' }, `${rangeLabel(range)}, from Cloudflare`)),
      !traffic ? h('div', { class: 'empty' }, 'Sonar can\'t be read.')
        : !traffic.length ? h('div', { class: 'empty' }, 'No traffic recorded in this range.')
          : h('div', { class: 'scroll-x' }, h('table', { class: 'table' },
            h('thead', null, h('tr', null, h('th', null, 'Site'), h('th', null, 'Project'), h('th', { class: 'num' }, 'Requests'), h('th', { class: 'num' }, 'Server errors'), h('th', { class: 'num' }, 'Sent'))),
            h('tbody', null, traffic.slice().sort((a, b) => b.requests - a.requests).map((t) => h('tr', null,
              h('td', { class: 'mono' }, t.host), h('td', null, t.project || '—'), h('td', { class: 'num' }, fmtCount(t.requests)),
              h('td', { class: `num ${t.errors_5xx ? 'red' : ''}` }, fmtCount(t.errors_5xx)), h('td', { class: 'num' }, fmtBytes(t.bytes))))))));
  }

  function sparkHealthCard(spark) {
    return h('article', { class: 'card panel' },
      panelHead('ϟ', 'Spark health'),
      h('div', { class: `big t-${spark.state}` }, '● ', LABEL[spark.state]),
      spark.reasons.length ? h('ul', { class: 'notes' }, spark.reasons.map((r) => h('li', null, r))) : null,
      spark.conditions.length
        ? h('div', { class: 'section' }, spark.conditions.map((c) => h('div', { class: 'attn' },
          h('i', { class: `dot ${c.level === 'critical' ? 'down' : 'warn'}` }),
          h('div', { class: 'attn-body' }, h('div', { class: 'attn-title' }, c.message)),
          h('div', { class: 'attn-meta', title: fmtFull(c.since) }, `since ${ago(c.since)}`))))
        : spark.state === 'healthy' ? h('p', { class: 'note' }, 'Sonar reports no conditions on the server.') : null,
      spark.collectors.length ? h('div', { class: 'chips' }, spark.collectors.map((c) => h('span', {
        class: `chip ${c.state === 'available' ? 'ok' : c.state === 'unsupported' ? 'off' : 'warn'}`,
        title: c.error || (c.last_ok ? `Last reading ${fmtFull(c.last_ok)}` : c.state),
      }, c.name))) : null);
  }

  function donut(t) {
    const parts = [['healthy', t.healthy, '#20d58a'], ['updating', t.updating, '#299dff'], ['degraded', t.degraded, '#ffbe45'], ['down', t.down, '#fc526b'], ['unknown', t.unknown, '#8397b0']];
    const r = 52, c = 2 * Math.PI * r;
    const svg = s('svg', { viewBox: '0 0 140 140', class: 'donut', role: 'img', 'aria-label': `${t.total} projects: ${parts.filter((p) => p[1]).map((p) => `${p[1]} ${p[0]}`).join(', ') || 'none'}` },
      s('circle', { cx: 70, cy: 70, r, stroke: '#20354e', 'stroke-width': 17, fill: 'none' }));
    let offset = 0;
    for (const [, n, color] of parts) {
      if (!n || !t.total) continue;
      const len = c * n / t.total;
      svg.append(s('circle', { cx: 70, cy: 70, r, stroke: color, 'stroke-width': 17, fill: 'none', 'stroke-dasharray': `${len} ${c - len}`, 'stroke-dashoffset': -offset, transform: 'rotate(-90 70 70)' }));
      offset += len;
    }
    svg.append(s('text', { x: 70, y: 72, 'font-size': 26, 'text-anchor': 'middle', 'font-weight': 700, fill: '#edf5ff' }, String(t.total)),
      s('text', { x: 70, y: 90, 'font-size': 12, 'text-anchor': 'middle', fill: '#c5d2e4' }, 'Projects'));
    return h('div', { class: 'donut-area' }, svg,
      h('div', { class: 'legend-list' }, parts.filter(([k, n]) => n || k !== 'updating').map(([k, n]) => h('div', null, badge(k, `${n} ${LABEL[k]}`)))));
  }

  // ---------- Overview ----------
  async function renderOverview({ page }) {
    const o = await loadOverview();
    const host = o.host, n = host.now;
    const cores = n && n.core_pcts ? n.core_pcts.length : 0;
    const pts = host.points;
    const net = mainInterface(host);
    const ctrs = o.containers;
    const running = ctrs ? ctrs.filter((c) => c.state === 'running').length : 0;
    const disk = n && n.disk;
    const rx = net.now ? net.now.rx_bytes_per_s : null, tx = net.now ? net.now.tx_bytes_per_s : null;

    const tiles = h('div', { class: 'grid grid-kpis' },
      tile('Spark health', 'pulse', h('span', { class: `t-${o.spark.state}` }, h('i', { class: `dot lg ${o.spark.state}` }), ' ', LABEL[o.spark.state]),
        o.spark.state !== 'unknown' && n ? `Uptime: ${fmtDuration(n.uptime_seconds)}` : (o.spark.reasons[0] || ''), null, '#/spark'),
      tile('Containers', 'box', ctrs ? `${running} / ${ctrs.length}` : '—', ctrs ? 'Running' : 'Sonar can\'t be read',
        ctrs && ctrs.length ? h('div', { class: 'progress' }, h('span', { style: { width: `${running / ctrs.length * 100}%`, background: 'var(--green)' } })) : null),
      tile('CPU usage', 'cpu', n ? fmtPct(n.cpu_pct) : '—', n && cores && isNum(n.cpu_pct) ? `(${(n.cpu_pct / 100 * cores).toFixed(1)} / ${cores} cores)` : '',
        sparkArea(pts.map((p) => (p.cpu_pct ? p.cpu_pct.avg : null)), COLORS.cpu)),
      tile('Memory usage', 'memory', n && n.memory_total ? fmtPct(n.memory_used_pct) : '—', n && n.memory_total ? `${fmtBytes(n.memory_used)} / ${fmtBytes(n.memory_total)}` : '',
        sparkArea(pts.map((p) => (p.mem_used_pct ? p.mem_used_pct.avg : null)), COLORS.mem)),
      tile('Disk usage', 'disk', disk ? fmtPct(disk.used_pct) : '—', disk ? `${fmtBytes(disk.used)} / ${fmtBytes(disk.total)}` : '',
        sparkArea(pts.map((p) => (isNum(p.disk_used_bytes) && p.disk_total_bytes ? p.disk_used_bytes / p.disk_total_bytes * 100 : null)), COLORS.disk)),
      tile('Network (total)', 'network', isNum(rx) && isNum(tx) ? fmtBits(rx + tx) : '—', isNum(rx) ? `↓ ${fmtBits(rx)}  ↑ ${fmtBits(tx)}` : '',
        sparkArea(net.series ? net.series.points.map((p) => (p.minutes ? (p.rx_bytes + p.tx_bytes) / (p.minutes * 60) : null)) : [], COLORS.rx)));

    return [
      heading(page, sourcesPill(o.sources)),
      tiles,
      h('div', { class: 'grid grid-2-1' },
        h('article', { class: 'card panel' }, panelHead('〽', 'System resource usage', h('span', { class: 'muted' }, rangeLabel(host.range))), hostChart(host, { height: 230 })),
        topContainersCard(ctrs, cores)),
      h('div', { class: 'grid grid-3' }, serverCard(host, ctrs), diskCard(host, o.disk, o.logs), networkCard(host)),
      containersCard(ctrs, cores),
      h('div', { class: 'grid grid-3' }, attentionPanel(o.attention),
        h('article', { class: 'card panel' }, panelHead('◇', 'Project status', h('a', { href: '#/projects' }, 'All projects')), donut(o.totals)),
        loadCard(n)),
      h('div', { class: 'grid grid-1-1' }, deploysPanel(host), errorsCard(o.errors, host.range)),
      h('p', { class: 'note' }, 'States, the server, containers and uptime come from Sonar; errors and logs from Blackbox; deploys from Lighthouse. ',
        'Anything a source can\'t say is shown as unknown or “—”, never as zero or healthy.'),
    ];
  }

  // cpuShare is a container's CPU as a share of the whole host, or of one core when the core count isn't known.
  const cpuShare = (c, cores) => (isNum(c.usage.cpu_cores) ? c.usage.cpu_cores / (cores || 1) * 100 : null);
  const netRate = (c) => (isNum(c.usage.net_rx_bytes_per_s) ? c.usage.net_rx_bytes_per_s + (c.usage.net_tx_bytes_per_s || 0) : null);

  function topContainersCard(ctrs, cores) {
    const body = h('div', { class: 'bars top-list' });
    const by = {
      cpu: { label: 'CPU usage', value: (c) => cpuShare(c, cores), fmt: (v) => fmtPct(v, 1) },
      memory: { label: 'Memory', value: (c) => c.usage.memory_bytes, fmt: fmtBytes },
      network: { label: 'Network', value: netRate, fmt: fmtBits },
    };
    const draw = () => {
      const m = by[state.topBy];
      const list = (ctrs || []).map((c) => ({ c, v: m.value(c) })).filter((x) => isNum(x.v)).sort((a, b) => b.v - a.v).slice(0, 5);
      const max = list.length ? list[0].v : 1;
      body.replaceChildren(...(list.length ? list.map(({ c, v }) => h('div', { class: 'top-row', title: `${c.project}/${c.service}` },
        h('button', { class: 'link-btn top-name', type: 'button', onclick: () => openProject(c.project) }, c.name || c.service),
        h('div', { class: 'bar-track' }, h('div', { class: 'bar-fill', style: { width: `${Math.max(2, v / max * 100)}%` } })),
        h('span', { class: 'bar-value' }, m.fmt(v))))
        : [h('div', { class: 'empty' }, ctrs ? 'Sonar has no measurements yet.' : 'Sonar can\'t be read.')]));
    };
    draw();
    const pick = h('select', { class: 'select', 'aria-label': 'Measure', onchange: (e) => { state.topBy = e.target.value; draw(); } },
      Object.entries(by).map(([k, m]) => h('option', { value: k, selected: k === state.topBy }, m.label)));
    return h('article', { class: 'card panel' }, panelHead('▥', h('span', null, 'Container resource usage ', h('span', { class: 'muted' }, '(Top 5)')), pick), body,
      h('p', { class: 'note' }, cores ? `CPU is a share of all ${cores} cores.` : 'CPU is a share of one core.'));
  }

  function containersCard(ctrs, cores) {
    const body = h('tbody');
    const count = h('span', { class: 'muted' });
    const draw = () => {
      const q = state.containerFilter.trim().toLowerCase();
      const rows = (ctrs || []).filter((c) => `${c.name} ${c.project} ${c.image}`.toLowerCase().includes(q))
        .filter((c) => state.containerStatus === 'all' || (state.containerStatus === 'running' ? c.state === 'running' && c.health !== 'unhealthy' : c.state !== 'running' || c.health === 'unhealthy'))
        .sort((a, b) => (a.state === 'running') - (b.state === 'running') || (cpuShare(b, cores) || 0) - (cpuShare(a, cores) || 0));
      count.textContent = ctrs ? plural(rows.length, 'container', 'containers') : '';
      body.replaceChildren(...(rows.length ? rows.map((c) => h('tr', { class: 'clickable', tabindex: 0, title: `Open ${c.project}`,
        onclick: () => openProject(c.project), onkeydown: (e) => { if (e.key === 'Enter') openProject(c.project); } },
      h('td', { class: 'name-cell' }, c.name || c.service),
      h('td', { class: 'muted' }, c.project),
      h('td', null, containerBadge(c)),
      h('td', { class: 'mono muted clip', title: c.image }, c.image),
      h('td', { class: 'num' }, fmtPct(cpuShare(c, cores), 1)),
      h('td', { class: 'num' }, fmtBytes(c.usage.memory_bytes), c.memory_limit ? h('span', { class: 'muted' }, ` / ${fmtBytes(c.memory_limit)}`) : null),
      h('td', { class: 'num' }, isNum(c.usage.net_rx_bytes_per_s) ? `${fmtBits(c.usage.net_rx_bytes_per_s)} / ${fmtBits(c.usage.net_tx_bytes_per_s)}` : '—'),
      h('td', { class: 'num' }, isNum(c.usage.blk_read_bytes_per_s) ? `${fmtRate(c.usage.blk_read_bytes_per_s)} / ${fmtRate(c.usage.blk_write_bytes_per_s)}` : '—'),
      h('td', { class: `num ${c.restart_count ? 'yellow' : ''}` }, String(c.restart_count)),
      h('td', { class: 'num muted' }, c.state === 'running' ? (c.started_at ? ago(c.started_at).replace(' ago', '') : '—') : (c.finished_at ? `stopped ${ago(c.finished_at)}` : 'stopped'))))
        : [h('tr', null, h('td', { colspan: 10, class: 'empty' }, ctrs ? 'No containers match.' : 'Sonar can\'t be read.'))]));
    };
    draw();
    const search = h('label', { class: 'search-box small-box' }, icon('search', 15),
      h('input', { type: 'search', placeholder: 'Search containers…', 'aria-label': 'Search containers', value: state.containerFilter,
        oninput: (e) => { state.containerFilter = e.target.value; draw(); } }));
    const status = h('select', { class: 'select', 'aria-label': 'Container status', onchange: (e) => { state.containerStatus = e.target.value; draw(); } },
      [['all', 'All statuses'], ['running', 'Running'], ['problem', 'Stopped or unhealthy']].map(([v, l]) => h('option', { value: v, selected: v === state.containerStatus }, l)));
    return h('article', { class: 'card panel grid' },
      panelHead('◇', h('span', null, 'Containers ', count), h('div', { class: 'panel-tools' }, search, status)),
      h('div', { class: 'scroll-x' }, h('table', { class: 'table containers' },
        h('thead', null, h('tr', null, ['Name', 'Project', 'Status', 'Image'].map((t) => h('th', null, t)),
          [['CPU', 'Share of all cores'], ['Memory', 'Used / limit'], ['Network', 'In / out'], ['Disk I/O', 'Read / write'], ['Restarts', 'Since the container was created'], ['Uptime', '']]
            .map(([t, tip]) => h('th', { class: 'num', title: tip }, t)))),
        body)));
  }

  function loadCard(n) {
    return h('article', { class: 'card panel' }, panelHead('▣', 'System load'),
      n ? [
        h('div', { class: 'eyebrow' }, 'Load average'),
        h('div', { class: 'load-grid' }, [['1 min', n.load1], ['5 min', n.load5], ['15 min', n.load15]].map(([l, v]) => h('div', null,
          h('div', { class: `big ${n.core_pcts && v > n.core_pcts.length ? 'yellow' : ''}` }, v.toFixed(2)), h('div', { class: 'small' }, l)))),
        n.core_pcts && n.core_pcts.length ? [
          h('div', { class: 'eyebrow' }, `Each of the ${n.core_pcts.length} cores now`),
          h('div', { class: 'cores' }, n.core_pcts.map((c, i) => h('span', { class: 'core', title: `Core ${i + 1}: ${fmtPct(c, 1)}` },
            h('span', { style: { height: `${Math.max(3, Math.min(100, c))}%` } })))),
        ] : null,
        n.swap_total ? meter('Swap', n.swap_used / n.swap_total * 100, `${fmtBytes(n.swap_used)} / ${fmtBytes(n.swap_total)}`) : kv('Swap', 'None'),
      ] : h('div', { class: 'empty' }, 'Sonar hasn\'t read the host yet, or can\'t be read.'));
  }

  function errorsCard(errors, range) {
    const head = panelHead('⚠', `Errors, ${rangeLabel(range).toLowerCase()}`, h('a', { href: hashFor('logs', { level: 'error', range }) }, 'Error lines in Logs'));
    if (!errors) return h('article', { class: 'card panel' }, head, h('div', { class: 'empty' }, 'Blackbox can\'t be read.'));
    const totals = errors.totals.filter((t) => t.error > 0).sort((a, b) => b.error - a.error).slice(0, 6);
    const max = totals.length ? totals[0].error : 1;
    return h('article', { class: 'card panel' }, head,
      totals.length ? h('div', { class: 'bars top-list' }, totals.map((t) => h('div', { class: 'top-row' },
        t.project ? h('button', { class: 'link-btn top-name', type: 'button', onclick: () => openProject(t.project) }, t.project) : h('span', { class: 'top-name muted' }, 'no project'),
        h('div', { class: 'bar-track' }, h('div', { class: 'bar-fill bad', style: { width: `${Math.max(2, t.error / max * 100)}%` } })),
        h('span', { class: 'bar-value' }, `${fmtCount(t.error)}${t.warn ? ` · ${fmtCount(t.warn)} warn` : ''}`))))
        : h('div', { class: 'all-clear' }, h('i', { class: 'dot healthy' }), 'No error lines in this range.'),
      errors.groups && errors.groups.length ? [
        h('div', { class: 'eyebrow section-gap' }, `Repeated errors${errors.total_groups > errors.groups.length ? `, the ${errors.groups.length} most frequent of ${fmtCount(errors.total_groups)}` : ''}`),
        errors.groups.slice(0, 5).map((g) => h('div', { class: 'attn' },
          h('span', { class: 'count-pill' }, `×${fmtCount(g.count)}`),
          h('div', { class: 'attn-body' }, h('a', { href: hashFor('logs', { around: g.newest_id }), class: 'attn-title mono-ish' }, g.fingerprint),
            h('div', { class: 'attn-detail' }, `${g.project || g.service} · first ${ago(g.first_seen)}`)),
          h('div', { class: 'attn-meta', title: fmtFull(g.last_seen) }, ago(g.last_seen)))),
      ] : null);
  }

  function progress(t) {
    const bar = h('div', { class: 'progress', role: 'img', 'aria-label': `${t.healthy} of ${t.total} healthy` });
    if (!t.total) return bar;
    for (const [k, color] of [['healthy', '#20d58a'], ['updating', '#299dff'], ['degraded', '#ffbe45'], ['down', '#fc526b'], ['unknown', '#8397b0']]) {
      if (t[k]) bar.append(h('span', { style: { width: `${t[k] / t.total * 100}%`, background: color } }));
    }
    return bar;
  }

  function storageBar(l) {
    const pct = Math.min(100, l.stored_bytes / l.max_bytes * 100);
    const fill = pct > 90 ? '#fc526b' : pct > 75 ? '#ffbe45' : '#20d58a';
    return h('div', { class: 'progress', title: `${pct.toFixed(0)}% of Blackbox's cap` }, h('span', { style: { width: `${pct}%`, background: fill } }));
  }

  const rangeLabel = (r) => ({ '1h': 'Last hour', '24h': 'Last 24 hours', '7d': 'Last 7 days', '30d': 'Last 30 days' }[r] || r);

  function attentionPanel(items) {
    const body = items.length
      ? items.slice(0, 12).map((a) => h('div', { class: 'attn' },
        h('i', { class: `dot ${a.level}`, title: LABEL[a.level] || a.level }),
        h('div', { class: 'attn-body' },
          h('div', { class: 'attn-title' }, a.link ? h('a', { href: a.link }, a.title) : a.title),
          a.detail ? h('div', { class: 'attn-detail' }, a.detail) : null),
        h('div', { class: 'attn-meta' }, a.source, a.at ? h('div', { title: fmtFull(a.at) }, ago(a.at)) : null)))
      : h('div', { class: 'all-clear' }, h('i', { class: 'dot healthy' }), 'Nothing needs attention. Every source is answering and every project is healthy.');
    return h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('♧'), 'Needs attention'),
        items.length > 12 ? h('span', { class: 'muted' }, `${items.length - 12} more`) : null),
      body);
  }

  function deploysPanel(host) {
    const list = host.deploys.slice().reverse().slice(0, 8);
    return h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('⇪'), 'Deploys'),
        h('a', { href: hashFor('activity', {}) }, 'Activity')),
      list.length ? list.map((d) => h('div', { class: 'list-row' },
        h('div', { class: 'list-main' },
          h('a', { href: `#/projects/${encodeURIComponent(d.project)}` }, d.project), ' ',
          h('span', { class: 'mono' }, d.to || 'unknown version'),
          h('small', null, d.from ? `from ${d.from} · ` : '', d.step ? `failed at ${d.step} · ` : '', `took ${fmtSeconds(d.seconds)}`)),
        h('div', { class: 'attn-meta' }, outcomeBadge(d.outcome),
          h('div', { title: fmtFull(d.started) }, fmtTime(d.started)))))
        : h('div', { class: 'empty' }, `No deploys in the ${rangeLabel(host.range).toLowerCase()}.`),
      h('p', { class: 'note' }, host.deploy_source === 'Lighthouse'
        ? 'Lighthouse\'s deploy records.'
        : 'Container swaps Sonar saw: Lighthouse can\'t be read, so deploys that failed before a swap aren\'t here. ',
      host.deploy_source === 'Lighthouse' ? null : h('a', { href: hashFor('logs', { project: 'lighthouse', q: 'deploy', range: '7d' }) }, 'Lighthouse\'s deploy log')));
  }

  const outcomeBadge = (o) => badge(o === 'ok' ? 'healthy' : 'down', o === 'ok' ? 'OK' : o);

  // Lighthouse's own word for a project, shown as a state.
  const LH_STATE = { ok: ['healthy', 'Deploying normally'], failing: ['degraded', 'Deploys failing'], broken: ['down', 'Broken'],
    stopped: ['unknown', 'Stopped'], not_deployed: ['unknown', 'Not deployed yet'] };
  const lhBadge = (st) => badge(...(LH_STATE[st] || ['unknown', st]));

  function projectsTable(projects, { title, compact, grouped, filter } = {}) {
    let rows = projects;
    if (filter) rows = rows.filter(filter);
    const tr = (p) => h('tr', {
      class: 'clickable', tabindex: 0, title: `Open ${p.name}`,
      onclick: () => openProject(p.name),
      onkeydown: (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openProject(p.name); } },
    },
    h('td', { class: 'name-cell' }, p.name),
    h('td', null, projectBadge(p)),
    h('td', { class: 'mono' }, p.version || '—'),
    compact ? null : h('td', { class: 'muted' }, p.monitoring),
    h('td', { class: 'num' }, isNum(p.response_ms) ? `${p.response_ms} ms` : '—'),
    h('td', { class: 'num' }, fmtUptime(p.uptime_30d), h('div', { class: 'cov' }, `${Math.round(p.uptime_30d.coverage_pct)}% observed`)),
    compact ? null : h('td', { class: `num ${p.errors && p.errors.error ? 'red' : ''}`, title: p.errors ? `${p.errors.warn} warnings` : 'Blackbox can\'t be read' },
      p.errors ? fmtCount(p.errors.error) : '—'),
    h('td', null, dayStrip(p.days.slice(-30))));

    const head = h('tr', null, h('th', null, 'Project'), h('th', null, 'Status'), h('th', null, 'Version'),
      compact ? null : h('th', null, 'Monitoring'), h('th', { class: 'num' }, 'Response'), h('th', { class: 'num' }, 'Uptime 30d'),
      compact ? null : h('th', { class: 'num', title: `Error lines, ${rangeLabel(state.range).toLowerCase()}` }, 'Errors'),
      h('th', null, 'Last 30 days'));
    const cols = compact ? 6 : 8;
    let body;
    if (!rows.length) {
      body = [h('tr', null, h('td', { colspan: cols, class: 'empty' }, projects.length ? 'No projects match.' : 'No projects: Sonar can\'t be read.'))];
    } else if (grouped) {
      const apps = rows.filter((p) => !p.infrastructure), infra = rows.filter((p) => p.infrastructure);
      body = [
        apps.length ? h('tr', { class: 'group' }, h('td', { colspan: cols }, 'Apps')) : null, apps.map(tr),
        infra.length ? h('tr', { class: 'group' }, h('td', { colspan: cols }, 'Infrastructure')) : null, infra.map(tr),
      ];
    } else {
      body = rows.map(tr);
    }
    return h('article', { class: 'card panel' },
      title ? h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('◇'), title)) : null,
      h('div', { class: 'scroll-x' }, h('table', { class: 'table' }, h('thead', null, head), h('tbody', null, body))));
  }

  // ---------- Projects ----------
  async function renderProjects({ page }) {
    const o = await loadOverview();
    const tableHost = h('div');
    const draw = () => {
      const q = state.projectFilter.trim().toLowerCase();
      tableHost.replaceChildren(projectsTable(o.projects, {
        grouped: true,
        filter: (p) => p.name.toLowerCase().includes(q) && (state.projectStatus === 'all' || p.state === state.projectStatus),
      }));
    };
    draw();
    const search = h('input', { class: 'input', type: 'search', placeholder: 'Filter projects…', 'aria-label': 'Filter projects', value: state.projectFilter,
      oninput: (e) => { state.projectFilter = e.target.value; draw(); } });
    const status = h('select', { class: 'select', 'aria-label': 'Status', onchange: (e) => { state.projectStatus = e.target.value; draw(); } },
      ['all', 'healthy', 'updating', 'degraded', 'down', 'unknown'].map((v) => h('option', { value: v, selected: v === state.projectStatus }, v === 'all' ? 'All states' : LABEL[v])));
    return [heading(page, sourcesPill(o.sources)), h('div', { class: 'filters' }, search, status), tableHost,
      h('p', { class: 'note' }, '“Outside check” means Sonar checks the project\'s public address; “containers only” means it only knows the containers are running. ',
        'Uptime leaves out time nobody observed; the share observed is shown under it.')];
  }

  async function openProject(name) {
    const back = $('#drawerBackdrop');
    $('#drawerName').textContent = name;
    const content = $('#drawerContent');
    content.replaceChildren(h('div', { class: 'loading' }, 'Loading…'));
    back.hidden = false;
    $('#drawerClose').focus();
    let o = state.overview;
    try {
      if (!o) o = await loadOverview();
    } catch (err) {
      content.replaceChildren(h('div', { class: 'error-box' }, err.message));
      return;
    }
    const p = o.projects.find((x) => x.name === name);
    if (!p) {
      content.replaceChildren(h('div', { class: 'empty' }, `Sonar doesn't report a project called “${name}”.`));
      return;
    }
    const errorsHost = h('div', null, h('div', { class: 'loading' }, 'Loading errors…'));
    const usageHost = h('div', null, h('div', { class: 'loading' }, 'Loading…'));
    const releasesHost = h('div', { class: 'section' });
    const days = p.days.filter((d) => d.state !== 'no_data');
    const downDays = p.days.filter((d) => d.down_minutes > 0).length;
    content.replaceChildren(
      h('div', { class: 'card' },
        h('div', { class: 'list-row' }, h('div', null, projectBadge(p), h('small', null, p.infrastructure ? 'Infrastructure' : 'App', ' · monitored by ', p.monitoring)),
          h('span', { class: 'pill mono' }, p.version || 'version unknown'))),
      section('Availability',
        h('div', { class: 'card' },
          kv('Uptime, 30 days', `${fmtUptime(p.uptime_30d)} (${Math.round(p.uptime_30d.coverage_pct)}% observed)`),
          kv('Uptime, 90 days', `${fmtUptime(p.uptime_90d)} (${Math.round(p.uptime_90d.coverage_pct)}% observed)`),
          kv('Response time', isNum(p.response_ms) ? `${p.response_ms} ms, last hour`
            : p.monitoring === 'outside check' ? 'No successful check in the last hour' : 'No outside check'),
          kv('Days with downtime', `${downDays} of the ${days.length} observed`)),
        h('div', { class: 'section' }, dayStrip(p.days, true), h('div', { class: 'note' }, `The last ${p.days.length} UTC days. Hover a day for its minutes.`))),
      section('Deploys', h('div', { class: 'card' },
        p.lighthouse
          ? [kv('Lighthouse', lhBadge(p.lighthouse.state)),
            kv('Deployed', p.lighthouse.version ? `${p.lighthouse.version}${p.lighthouse.deployed_at ? `, ${fmtFull(p.lighthouse.deployed_at)}` : ''}` : '—'),
            kv('Follows', p.lighthouse.mode === 'releases' ? 'Version tags' : 'Its branch'),
            p.lighthouse.last_error ? kv('Last problem', `${p.lighthouse.last_error}${p.lighthouse.last_error_at ? ` (${ago(p.lighthouse.last_error_at)})` : ''}`) : null]
          : kv('Lighthouse', h('span', { class: 'muted' }, 'Not reported')),
        p.last_deploy
          ? [kv('Last deploy, 24 hours', outcomeBadge(p.last_deploy.outcome)),
            kv('Version', `${p.last_deploy.from ? `${p.last_deploy.from} → ` : ''}${p.last_deploy.to || 'unknown'}${p.last_deploy.step ? `, failed at ${p.last_deploy.step}` : ''}`),
            kv('When', `${fmtFull(p.last_deploy.started)} (took ${fmtSeconds(p.last_deploy.seconds)}, from ${p.last_deploy.source})`)]
          : kv('Last deploy, 24 hours', 'None')),
        releasesHost),
      p.checks && p.checks.length ? section('Outside checks', h('div', { class: 'card' }, p.checks.map((c) => h('div', { class: 'list-row' },
        h('div', { class: 'list-main' }, h('span', { class: 'mono' }, c.url),
          h('small', null, c.last_at ? `${c.last_status ? `HTTP ${c.last_status}` : 'No answer'}${isNum(c.last_latency_ms) ? `, ${c.last_latency_ms} ms` : ''}, ${ago(c.last_at)}` : 'Not checked yet',
            c.consecutive_failures ? ` · ${plural(c.consecutive_failures, 'failure', 'failures')} in a row` : ''),
          c.last_error ? h('small', { class: 't-down' }, c.last_error) : null),
        badge(c.state === 'operational' ? 'healthy' : c.state))))) : null,
      section('Containers', containerList(o.containers, name)),
      section(`Resources, ${rangeLabel(state.range).toLowerCase()}`, usageHost),
      storageSection(p),
      p.traffic && p.traffic.length ? section(`Traffic, ${rangeLabel(o.traffic_range).toLowerCase()}`, h('div', { class: 'card' }, p.traffic.map((t) =>
        kv(t.host, `${fmtCount(t.requests)} requests, ${fmtCount(t.errors_5xx)} server errors, ${fmtBytes(t.bytes)} sent`)))) : null,
      section(`Errors, ${rangeLabel(state.range).toLowerCase()}`,
        p.errors ? h('div', { class: 'card' }, kv('Error lines', h('span', { class: p.errors.error ? 'red' : '' }, fmtCount(p.errors.error))), kv('Warnings', fmtCount(p.errors.warn))) : null,
        errorsHost),
      appSection(p),
      h('div', { class: 'log-actions' },
        h('a', { href: hashFor('logs', { project: name, range: '24h' }) }, 'Open its logs'),
        h('a', { href: hashFor('activity', { project: name }) }, 'Its activity')),
      h('p', { class: 'note' }, 'Read-only. Events close together are leads to investigate, not proof of cause.'),
    );
    loadReleases(name, releasesHost);
    loadUsage(name, usageHost);
    try {
      const page = await api(`/api/v1/logs?${new URLSearchParams({ project: name, level: 'error', range: '24h', limit: '8' })}`);
      errorsHost.replaceChildren(page.records.length
        ? [h('div', { class: 'logs' }, page.records.map((r) => logRow(r, { compact: true }))),
          h('div', { class: 'pager' }, h('a', { href: hashFor('logs', { project: name, level: 'error', range: '24h' }) }, 'All errors in Logs'))]
        : h('div', { class: 'card muted' }, 'No error lines in the last 24 hours.'));
    } catch (err) {
      errorsHost.replaceChildren(h('div', { class: 'card muted' }, err.message));
    }
  }

  function containerList(containers, project) {
    if (!containers) return h('div', { class: 'card muted' }, 'Sonar can\'t be read.');
    const list = containers.filter((c) => c.project === project);
    if (!list.length) return h('div', { class: 'card muted' }, 'Sonar reports no containers for it.');
    return h('div', { class: 'card' }, list.map((c) => h('div', { class: 'list-row' },
      h('div', { class: 'list-main' }, h('strong', null, c.name || c.service), ' ', h('span', { class: 'muted mono' }, c.image),
        h('small', null, `CPU ${fmtCores(c.usage.cpu_cores)} · memory ${fmtBytes(c.usage.memory_bytes)}${c.memory_limit ? ` of ${fmtBytes(c.memory_limit)}` : ''}`,
          ` · network ${fmtBits(c.usage.net_rx_bytes_per_s)} in, ${fmtBits(c.usage.net_tx_bytes_per_s)} out`),
        h('small', null, c.state === 'running' ? `Up ${c.started_at ? ago(c.started_at).replace(' ago', '') : '—'}` : `Stopped ${c.finished_at ? ago(c.finished_at) : ''}, exit code ${c.exit_code}`,
          c.restart_count ? ` · ${plural(c.restart_count, 'restart', 'restarts')}` : '')),
      containerBadge(c))));
  }

  function containerBadge(c) {
    if (c.state !== 'running') return badge(c.state === 'restarting' ? 'degraded' : 'down', c.state ? c.state[0].toUpperCase() + c.state.slice(1) : 'Unknown');
    if (c.health === 'unhealthy') return badge('down', 'Unhealthy');
    if (c.health === 'starting') return badge('degraded', 'Starting');
    return badge('healthy', 'Running');
  }

  function storageSection(p) {
    const rows = [];
    if (p.disk) {
      rows.push(kv('Data folder', isNum(p.disk.data_bytes) ? `${fmtBytes(p.disk.data_bytes)} in ${fmtCount(p.disk.data_files)} files` : 'None'));
      rows.push(kv('Images and containers', `${fmtBytes(p.disk.image_bytes)} and ${fmtBytes(p.disk.container_bytes)}`));
      if (p.disk.volume_bytes) rows.push(kv('Volumes', fmtBytes(p.disk.volume_bytes)));
    }
    for (const d of p.databases || []) rows.push(kv(`Database ${d.name}`, `${fmtBytes(d.size_bytes)}, ${plural(d.connections, 'connection', 'connections')}`));
    return rows.length ? section('Storage', h('div', { class: 'card' }, rows)) : null;
  }

  function appSection(p) {
    const a = p.app;
    if (!a) return section('Usage', h('div', { class: 'card muted' }, `Not instrumented: ${p.name} doesn't serve a report.`));
    return section('Usage, 30 days', h('div', { class: 'card' },
      a.counters.map((c) => kv(c.name.replace(/_/g, ' '), fmtCount(c.total))),
      a.checks.map((c) => kv(c.name.replace(/_/g, ' '), badge(c.ok ? 'healthy' : 'down', c.ok ? 'OK' : c.message))),
      a.reporters.filter((r) => r.error).map((r) => kv(`Report from ${r.container}`, h('span', { class: 't-down' }, r.error))),
      !a.counters.length && !a.checks.length ? h('div', { class: 'muted' }, 'Its report has no counters or checks.') : null));
  }

  async function loadUsage(name, host) {
    try {
      const u = await api(`/api/v1/projects/${encodeURIComponent(name)}/usage?range=${encodeURIComponent(state.range)}`);
      if (!u.services.length) { host.replaceChildren(h('div', { class: 'card muted' }, 'Sonar has no measurements for it in this range.')); return; }
      const palette = ['#30a7ff', '#ae75fc', '#20d58a', '#ffbe45', '#fc526b', '#27d3d3'];
      const series = (fn) => u.services.map((sv, i) => ({ name: sv.service, color: palette[i % palette.length], points: sv.points.map((p) => ({ t: Date.parse(p.t), v: fn(p) })) }));
      const cpu = series((p) => (p.cpu_cores ? p.cpu_cores.avg * 100 : null));
      const mem = series((p) => (p.memory_bytes ? p.memory_bytes.avg : null));
      const top = (ss) => niceMax(Math.max(0, ...ss.flatMap((x) => x.points.map((p) => p.v || 0))) * 1.15);
      const opts = { height: 130, from: Date.parse(u.from), to: Date.parse(u.to) };
      host.replaceChildren(h('div', { class: 'card' },
        h('div', { class: 'eyebrow' }, 'CPU, share of one core'),
        lineChart({ ...opts, series: cpu, yMax: top(cpu), label: `${name} CPU`, axis: (v) => `${Math.round(v)}%`, tipFmt: (v) => fmtPct(v, 1) }),
        h('div', { class: 'eyebrow' }, 'Memory'),
        lineChart({ ...opts, series: mem, yMax: top(mem), label: `${name} memory`, axis: fmtBytes, tipFmt: fmtBytes }),
        h('div', { class: 'legend' }, cpu.map((x) => h('span', null, h('i', { style: { background: x.color } }), x.name)))));
    } catch (err) {
      host.replaceChildren(h('div', { class: 'card muted' }, err.message));
    }
  }

  async function loadReleases(name, host) {
    try {
      const { releases } = await api(`/api/v1/projects/${encodeURIComponent(name)}/releases`);
      if (!releases.length) return;
      host.replaceChildren(h('h3', null, 'Releases'), h('div', { class: 'card' }, releases.slice(0, 8).map((r) => h('div', { class: 'list-row' },
        h('div', { class: 'list-main' }, h('span', { class: 'mono' }, r.tag), r.note ? h('small', null, r.note) : null),
        h('div', { class: 'attn-meta' }, r.status, h('div', { title: fmtFull(r.published_at || r.created_at) }, ago(r.published_at || r.created_at)))))));
    } catch {
      // Lighthouse not connected or not answering: the Sources page says which.
    }
  }

  function closeDrawer(updateHash = true) {
    const back = $('#drawerBackdrop');
    if (back.hidden) return;
    back.hidden = true;
    if (updateHash && parseHash().page.id === 'projects' && parseHash().arg) {
      history.replaceState(null, '', '#/projects');
    }
  }

  const section = (title, ...body) => h('section', { class: 'section' }, h('h3', null, title), body);
  const kv = (k, v) => h('div', { class: 'kv' }, h('span', null, k), h('span', null, v));

  // ---------- Spark ----------
  async function renderSpark({ page }) {
    const o = await loadOverview();
    const infraProjects = o.projects.filter((p) => p.infrastructure);
    return [
      heading(page, sourcesPill(o.sources)),
      h('div', { class: 'grid grid-3' }, sparkHealthCard(o.spark), serverCard(o.host, o.containers), infraCard(o.infrastructure)),
      h('article', { class: 'card panel grid' },
        panelHead('〽', 'Host resources', h('span', { class: 'muted' }, `${rangeLabel(o.host.range)} · average, with each bucket's range shaded`)),
        hostChart(o.host, { height: 260, band: true })),
      h('div', { class: 'grid grid-3' }, diskCard(o.host, o.disk, o.logs), networkCard(o.host), databasesCard(o.databases)),
      h('div', { class: 'grid grid-1-1' }, diskByProjectCard(o.disk), logsCard(o.logs)),
      trafficCard(o.traffic, o.traffic_range),
      projectsTable(infraProjects, { title: 'Infrastructure projects' }),
    ];
  }

  function infraCard(infra) {
    return h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('◉'), 'Infrastructure')),
      infra.map((i) => h('div', { class: 'list-row' },
        h('div', { class: 'list-main' }, h('strong', null, i.name),
          h('small', null, i.reported ? (i.version ? `Version ${i.version}` : 'Version unknown') : 'Sonar doesn\'t report it yet')),
        i.reported ? badge(i.state) : badge('not_connected', 'Not reported'))),
      h('p', { class: 'note' }, 'As Sonar reports them. Vista runs no checks of its own.'));
  }

  function logsCard(l) {
    return h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('▤'), 'Log collection')),
      l ? [
        kv('Stored', `${fmtBytes(l.stored_bytes)} of ${fmtBytes(l.max_bytes)}`),
        l.max_bytes ? storageBar(l) : null,
        kv('Oldest line', l.oldest ? `${fmtFull(l.oldest)} (${ago(l.oldest)})` : '—'),
        kv('Collector lag', fmtSeconds(l.lag_seconds)),
        kv('Streams', `${l.streams_open} open${l.streams_refused ? `, ${l.streams_refused} refused` : ''}`),
        kv('Gaps, 24 hours', String(l.gaps_24h)),
        kv('Blackbox version', l.version || '—'),
      ] : h('div', { class: 'empty' }, 'Blackbox can\'t be read.'));
  }

  // ---------- Activity ----------
  async function renderActivity({ page, params }) {
    const range = params.get('range') || state.range;
    const a = await api(`/api/v1/activity?range=${encodeURIComponent(range)}`);
    setUpdated(a.generated_at);
    const project = params.get('project') || '';
    const kind = params.get('kind') || '';
    const events = a.events.filter((e) => (!project || e.project === project) && (!kind || e.kind === kind));
    const nav = (changes) => { location.hash = hashFor('activity', { range: params.get('range'), project, kind, ...changes }); };
    const kinds = { deploy: 'Deploy', deploy_failed: 'Failed deploy', state: 'Health change', log_gap: 'Log gap' };
    const projects = project && !a.projects.includes(project) ? [...a.projects, project] : a.projects;
    return [
      heading(page),
      h('div', { class: 'filters' },
        h('select', { class: 'select', 'aria-label': 'Project', onchange: (e) => nav({ project: e.target.value }) },
          h('option', { value: '' }, 'All projects'), projects.map((p) => h('option', { value: p, selected: p === project }, p))),
        h('select', { class: 'select', 'aria-label': 'Kind', onchange: (e) => nav({ kind: e.target.value }) },
          h('option', { value: '' }, 'Every kind'), Object.entries(kinds).map(([k, l]) => h('option', { value: k, selected: k === kind }, l))),
        h('span', { class: 'pill' }, rangeLabel(range)),
        h('a', { href: hashFor('logs', { project: 'lighthouse', q: 'deploy', range }) }, 'Lighthouse\'s deploy log')),
      sourceWarnings(a.sources),
      errorChartCard(a.errors, range, project),
      h('article', { class: 'card panel' },
        h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('⌁'), 'Timeline'),
          h('span', { class: 'muted' }, plural(events.length, 'event', 'events'))),
        events.length ? events.map((e) => h('div', { class: 'event' },
          h('div', { class: 'event-time', title: fmtFull(e.at) }, fmtTime(e.at)),
          h('div', null, h('span', { class: `kind ${e.kind}` }, kinds[e.kind] || e.kind)),
          h('div', null,
            h('div', { class: 'event-title' }, e.title),
            e.detail ? h('div', { class: 'event-detail' }, e.detail) : null,
            h('div', { class: 'log-actions' },
              e.link ? h('a', { href: e.link }, 'Logs around it') : null,
              e.deploy_id ? stepsToggle(e.deploy_id) : null)),
          h('div', { class: 'event-src' }, e.source)))
          : h('div', { class: 'empty' }, 'Nothing happened in this range, as far as the connected sources know.')),
      h('ul', { class: 'notes' }, a.notes.map((n) => h('li', null, n))),
    ];
  }

  // errorChartCard draws Blackbox's error and warn lines per bucket, stacked.
  function errorChartCard(errors, range, project) {
    const title = `Errors and warnings${project ? ` in ${project}` : ''}`;
    if (!errors) return h('article', { class: 'card panel grid' }, panelHead('▥', title), h('div', { class: 'empty' }, 'Blackbox can\'t be read.'));
    const span = rangeMs(range);
    const step = errors.bucket === 'day' ? 864e5 : 36e5;
    const end = Math.ceil(Date.now() / step) * step, start = end - Math.ceil(span / step) * step;
    const n = Math.round((end - start) / step);
    const err = new Array(n).fill(0), warn = new Array(n).fill(0);
    for (const b of errors.buckets) {
      if (project && b.project !== project) continue;
      const i = Math.floor((Date.parse(b.start) - start) / step);
      if (i >= 0 && i < n) { err[i] += b.error; warn[i] += b.warn; }
    }
    const totalErr = err.reduce((x, y) => x + y, 0), totalWarn = warn.reduce((x, y) => x + y, 0);
    return h('article', { class: 'card panel grid' },
      panelHead('▥', title, h('span', { class: 'muted' }, `${fmtCount(totalErr)} errors, ${fmtCount(totalWarn)} warnings · per ${errors.bucket}`)),
      barChart({ start, step, err, warn }),
      h('div', { class: 'legend' }, h('span', null, h('i', { style: { background: COLORS.err } }), 'Errors'), h('span', null, h('i', { style: { background: COLORS.warn } }), 'Warnings')));
  }

  function barChart({ start, step, err, warn, height = 120 }) {
    const wrap = h('div', { class: 'chart-wrap' });
    const draw = (W) => {
      const pad = { l: 40, r: 8, t: 8, b: 20 }, H = height, n = err.length;
      const top = niceMax(Math.max(1, ...err.map((e, i) => e + warn[i])));
      const bw = (W - pad.l - pad.r) / n;
      const Y = (v) => H - pad.b - (H - pad.t - pad.b) * v / top;
      const svg = s('svg', { class: 'chart', width: W, height: H, viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'Error and warning lines per bucket' });
      for (const f of [0, 0.5, 1]) svg.append(s('line', { class: 'grid-line', x1: pad.l, x2: W - pad.r, y1: Y(f * top), y2: Y(f * top) }),
        s('text', { x: pad.l - 6, y: Y(f * top) + 3, 'text-anchor': 'end' }, fmtCount(Math.round(f * top))));
      for (let i = 0; i < n; i++) {
        const x = pad.l + i * bw + bw * 0.15, w = Math.max(1, bw * 0.7);
        const label = `${fmtFull(new Date(start + i * step).toISOString())}: ${err[i]} errors, ${warn[i]} warnings`;
        if (err[i]) svg.append(s('rect', { x, y: Y(err[i]), width: w, height: Y(0) - Y(err[i]), fill: COLORS.err, rx: 1 }, s('title', null, label)));
        if (warn[i]) svg.append(s('rect', { x, y: Y(err[i] + warn[i]), width: w, height: Y(0) - Y(warn[i]), fill: COLORS.warn, rx: 1 }, s('title', null, label)));
      }
      const ticks = W < 500 ? 3 : 6;
      for (let k = 0; k <= ticks; k++) {
        const t = start + (n * step) * k / ticks;
        svg.append(s('text', { x: pad.l + (W - pad.l - pad.r) * k / ticks, y: H - 5, 'text-anchor': k === 0 ? 'start' : k === ticks ? 'end' : 'middle' }, tickLabel(t, n * step)));
      }
      wrap.replaceChildren(svg);
    };
    let lastW = 0;
    const ro = new ResizeObserver(() => {
      if (!wrap.isConnected) { ro.disconnect(); return; }
      const w = Math.floor(wrap.clientWidth);
      if (w > 0 && w !== lastW) { lastW = w; draw(w); }
    });
    ro.observe(wrap);
    return wrap;
  }

  const rangeMs = (r) => ({ '1h': 36e5, '24h': 864e5, '7d': 7 * 864e5, '30d': 30 * 864e5 }[r] || 864e5);

  // stepsToggle shows a Lighthouse deploy's steps, loaded when first opened.
  function stepsToggle(id) {
    const box = h('div', { class: 'steps', hidden: true });
    const btn = h('button', { class: 'link-btn', type: 'button', 'aria-expanded': 'false', onclick: async () => {
      box.hidden = !box.hidden;
      btn.setAttribute('aria-expanded', String(!box.hidden));
      if (box.hidden || box.childNodes.length) return;
      box.append(h('div', { class: 'loading' }, 'Loading steps…'));
      try {
        const d = await api(`/api/v1/deployments/${id}`);
        box.replaceChildren(...d.steps.map((st) => h('div', { class: 'step' },
          h('div', { class: 'list-row' }, h('span', { class: 'mono' }, st.step),
            h('span', null, badge(st.status === 'succeeded' ? 'healthy' : st.status === 'failed' ? 'down' : 'unknown', st.status), ' ',
              h('span', { class: 'muted' }, fmtSeconds((toDate(st.finished_at) - toDate(st.started_at)) / 1000)))),
          st.status === 'failed' && st.log ? h('pre', { class: 'step-log' }, st.log) : null)));
        if (!d.steps.length) box.replaceChildren(h('div', { class: 'muted' }, 'Lighthouse recorded no steps for this deploy.'));
      } catch (err) {
        box.replaceChildren(h('div', { class: 'muted' }, err.message));
      }
    } }, 'Steps');
    return [btn, box];
  }

  function sourceWarnings(sources) {
    const bad = sources.filter((x) => x.state === 'unavailable' || x.state === 'stale');
    if (!bad.length) return null;
    return h('div', { class: 'error-box grid' }, bad.map((x) => h('div', null, h('strong', null, `${x.name}: `), x.detail || LABEL[x.state])));
  }

  // ---------- Logs ----------
  const LOG_RANGES = ['1h', '24h', '7d', '30d'];

  async function renderLogs({ page, params }) {
    const filters = {
      range: params.get('range') || (params.get('from') ? '' : '1h'),
      from: params.get('from') || '', to: params.get('to') || '',
      project: params.get('project') || '', level: params.get('level') || '',
      q: params.get('q') || '', version: params.get('version') || '',
    };
    const around = params.get('around');
    let logSources = [];
    try { logSources = (await api('/api/v1/log-sources')).sources; } catch { /* the search below reports it */ }
    const projects = [...new Set(logSources.map((x) => x.project).filter(Boolean))].sort();
    if (filters.project && !projects.includes(filters.project)) projects.push(filters.project);

    const form = h('form', { class: 'filters', onsubmit: (e) => {
      e.preventDefault();
      const f = new FormData(e.target);
      const next = {};
      for (const k of ['range', 'project', 'level', 'q', 'version']) next[k] = String(f.get(k) || '').trim();
      if (next.range === 'custom') { next.range = ''; next.from = filters.from; next.to = filters.to; }
      location.hash = hashFor('logs', next);
    } },
    h('input', { class: 'input', type: 'search', name: 'q', placeholder: 'Text in the message or fields…', 'aria-label': 'Search text', value: filters.q, maxlength: 200 }),
    h('select', { class: 'select', name: 'project', 'aria-label': 'Project' },
      h('option', { value: '' }, 'All projects'), projects.map((p) => h('option', { value: p, selected: p === filters.project }, p))),
    h('select', { class: 'select', name: 'level', 'aria-label': 'Level' },
      [['', 'Every level'], ['error', 'Errors'], ['warn', 'Warnings and up'], ['info', 'Info and up'], ['debug', 'Debug and up']]
        .map(([v, l]) => h('option', { value: v, selected: v === filters.level }, l))),
    h('select', { class: 'select', name: 'range', 'aria-label': 'Time range' },
      !filters.range ? h('option', { value: 'custom', selected: true }, 'Chosen window') : null,
      LOG_RANGES.map((r) => h('option', { value: r, selected: r === filters.range }, rangeLabel(r)))),
    h('input', { class: 'input short', name: 'version', placeholder: 'Version', 'aria-label': 'Version', value: filters.version, maxlength: 100 }),
    h('button', { class: 'btn', type: 'submit' }, 'Search'));

    const out = [heading(page), form];
    if (filters.from || filters.to) {
      out.push(h('p', { class: 'note' }, `Window: ${filters.from ? fmtFull(filters.from) : 'start'} to ${filters.to ? fmtFull(filters.to) : 'now'}.`));
    }
    if (around) out.push(await contextPanel(around));

    const results = h('div', { class: 'logs' });
    const footer = h('div', { class: 'pager' });
    const summary = h('span', { class: 'muted' });
    const query = { limit: '100' };
    for (const k of ['range', 'from', 'to', 'project', 'level', 'q', 'version']) if (filters[k]) query[k] = filters[k];
    let count = 0;
    const loadPage = async (cursor) => {
      const q = new URLSearchParams(query);
      if (cursor) q.set('cursor', cursor);
      const res = await api(`/api/v1/logs?${q}`);
      count += res.records.length;
      results.append(...res.records.map((r) => logRow(r)));
      summary.textContent = `${plural(count, 'line', 'lines')}, newest first${res.unknown_left_out ? ` · ${res.unknown_left_out} lines without a level left out` : ''}`;
      footer.replaceChildren(res.next_cursor
        ? h('button', { class: 'btn', type: 'button', onclick: async (e) => {
          e.target.disabled = true;
          try { await loadPage(res.next_cursor); } catch (err) { footer.replaceChildren(h('div', { class: 'error-box' }, err.message)); }
        } }, 'Load older lines')
        : (count ? h('span', { class: 'muted' }, 'End of the results.') : null));
      if (!count) results.replaceChildren(h('div', { class: 'empty' }, 'No lines match.'));
    };
    try {
      await loadPage('');
    } catch (err) {
      return [...out, h('div', { class: 'error-box' }, err.message)];
    }
    out.push(h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('▤'), 'Lines'), summary),
      results, footer,
      h('p', { class: 'note' }, 'Select a line for its fields and the lines around it. Times are your local time.')));
    return out;
  }

  async function contextPanel(id) {
    try {
      const c = await api(`/api/v1/logs/${encodeURIComponent(id)}/context?before=30&after=30`);
      return h('article', { class: 'card panel grid' },
        h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('⌖'), 'Line in context'),
          h('span', { class: 'muted' }, 'Same container, 30 lines either side')),
        h('div', { class: 'logs' }, c.records.map((r) => logRow(r, { target: r.id === c.target }))));
    } catch (err) {
      return h('div', { class: 'error-box grid' }, err.message);
    }
  }

  function logRow(r, { target = false, compact = false } = {}) {
    const lvl = r.level || 'unknown';
    const where = `${r.project || r.container}${r.service && r.service !== r.project ? `/${r.service}` : ''}`;
    const more = h('div', { class: 'log-more', hidden: true });
    const row = h('div', { class: `log${target ? ' target' : ''}` },
      h('button', { class: 'log-line', type: 'button', 'aria-expanded': 'false', onclick: (e) => {
        const btn = e.currentTarget;
        const open = more.hidden;
        more.hidden = !open;
        btn.setAttribute('aria-expanded', String(open));
        if (open && !more.childNodes.length) fillDetails(more, r);
      } },
      h('span', { class: 'log-ts', title: fmtFull(r.ts) }, compact ? fmtTime(r.ts) : new Date(r.ts).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })),
      h('span', { class: `lvl-${lvl}` }, lvl.toUpperCase()),
      h('span', { class: 'log-src', title: where }, where),
      h('span', { class: 'log-msg' }, r.message, r.truncated ? h('span', { class: 'muted' }, ' [cut short]') : null)),
      more);
    return row;
  }

  function fillDetails(el, r) {
    let attrs = '';
    if (r.attrs != null) {
      try { attrs = JSON.stringify(r.attrs, null, 2); } catch { attrs = String(r.attrs); }
    }
    const ctx = h('div', { class: 'context' });
    add(el, [
      kv('Container', `${r.container} (${r.stream})`),
      kv('Version', r.version || '—'),
      kv('Time', `${fmtFull(r.ts)} · ${r.ts} UTC`),
      r.redacted ? kv('Note', 'Blackbox hid a secret in this line.') : null,
      attrs ? h('pre', null, attrs) : null,
      h('div', { class: 'log-actions' },
        h('button', { class: 'link-btn', type: 'button', onclick: async (e) => {
          e.target.disabled = true;
          ctx.replaceChildren(h('div', { class: 'loading' }, 'Loading…'));
          try {
            const c = await api(`/api/v1/logs/${r.id}/context?before=20&after=20`);
            ctx.replaceChildren(h('div', { class: 'logs' }, c.records.map((x) => logRow(x, { target: x.id === c.target }))));
          } catch (err) {
            ctx.replaceChildren(h('div', { class: 'error-box' }, err.message));
          }
        } }, 'Show the lines around it'),
        h('a', { href: hashFor('logs', { around: r.id }) }, 'Link to this line'),
        r.project ? h('a', { href: hashFor('activity', { project: r.project }) }, `${r.project}'s activity`) : null),
      ctx]);
  }

  // ---------- Incidents ----------
  async function renderIncidents({ page }) {
    const inc = await api('/api/v1/incidents');
    setUpdated(inc.generated_at);
    return [
      heading(page, inc.overall ? h('span', { class: 'pill' }, `Public projects, 30 days: ${fmtUptime(inc.overall)} up`) : null),
      sourceWarnings(inc.sources),
      h('article', { class: 'card panel grid' },
        panelHead('⚠', 'Incidents, last 30 days', inc.incidents ? h('span', { class: 'muted' }, plural(inc.incidents.length, 'incident', 'incidents')) : null),
        !inc.incidents ? h('div', { class: 'empty' }, 'Sonar\'s state changes can\'t be read.')
          : !inc.incidents.length ? h('div', { class: 'empty' }, 'No project was down or degraded in the last 30 days.')
            : h('div', { class: 'scroll-x' }, h('table', { class: 'table' },
              h('thead', null, h('tr', null, h('th', null, 'Began'), h('th', null, 'Project'), h('th', null, 'State'), h('th', { class: 'num' }, 'Lasted'), h('th', null, 'What wasn\'t up'))),
              h('tbody', null, inc.incidents.map((i) => h('tr', { class: 'clickable', onclick: () => openProject(i.project) },
                h('td', { class: 'mono', title: `${fmtFull(i.start)} to ${fmtFull(i.end)}` }, fmtTime(i.start)),
                h('td', { class: 'name-cell' }, i.project), h('td', null, badge(i.state)),
                h('td', { class: 'num' }, fmtSeconds((Date.parse(i.end) - Date.parse(i.start)) / 1000)),
                h('td', { class: 'muted wrap' }, i.detail))))))),
      h('div', { class: 'grid grid-1-1' },
        h('article', { class: 'card panel' },
          h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('⚠'), 'Days with trouble'),
            h('span', { class: 'muted' }, `${inc.days.length} in 90 days`)),
          inc.days.length ? h('div', { class: 'scroll-x' }, h('table', { class: 'table' },
            h('thead', null, h('tr', null, h('th', null, 'Day (UTC)'), h('th', null, 'Project'), h('th', null, 'Worst'),
              h('th', { class: 'num' }, 'Down'), h('th', { class: 'num' }, 'Degraded'), h('th', { class: 'num' }, 'Uptime'))),
            h('tbody', null, inc.days.map((d) => h('tr', { class: 'clickable', onclick: () => openProject(d.project) },
              h('td', { class: 'mono' }, d.date), h('td', { class: 'name-cell' }, d.project), h('td', null, badge(d.state)),
              h('td', { class: 'num' }, d.down_minutes ? fmtMinutes(d.down_minutes) : '—'),
              h('td', { class: 'num' }, d.degraded_minutes ? fmtMinutes(d.degraded_minutes) : '—'),
              h('td', { class: 'num' }, isNum(d.uptime_pct) ? `${d.uptime_pct.toFixed(2)}%` : '—',
                h('div', { class: 'cov' }, `${Math.round(d.coverage_pct)}% observed`)))))))
            : h('div', { class: 'empty' }, inc.availability.length ? 'No project was down or degraded in the last 90 days.' : 'Sonar can\'t be read.')),
        h('article', { class: 'card panel' },
          h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, iconFor('◇'), 'Availability')),
          h('div', { class: 'scroll-x' }, h('table', { class: 'table' },
            h('thead', null, h('tr', null, h('th', null, 'Project'), h('th', null, 'Monitoring'), h('th', { class: 'num' }, '30 days'), h('th', { class: 'num' }, '90 days'))),
            h('tbody', null, inc.availability.map((a) => h('tr', { class: 'clickable', onclick: () => openProject(a.project) },
              h('td', { class: 'name-cell' }, a.project), h('td', { class: 'muted' }, a.monitoring),
              h('td', { class: 'num' }, fmtUptime(a.uptime_30d), h('div', { class: 'cov' }, `${Math.round(a.uptime_30d.coverage_pct)}% observed`)),
              h('td', { class: 'num' }, fmtUptime(a.uptime_90d), h('div', { class: 'cov' }, `${Math.round(a.uptime_90d.coverage_pct)}% observed`))))))))),
      h('ul', { class: 'notes' }, inc.notes.map((n) => h('li', null, n))),
    ];
  }

  // ---------- Sources ----------
  const SOURCE_INFO = {
    sonar: { gives: 'Each project\'s state, version, uptime, history and state changes; the host\'s CPU, memory, swap, load, disk and network; '
      + 'every container with its use and restarts; outside checks; sparkdb\'s connections and sizes; disk use per project; and traffic per site from Cloudflare.' },
    blackbox: { gives: 'Log search, the lines around a line, error counts per project, repeated errors, log sources, gaps and collection health.' },
    lighthouse: { gives: 'Deploy records with their steps, rollbacks, releases, and each project\'s deploy state.' },
    apps: { gives: 'Counters and checks each application reports, such as games played or commands run, collected by Sonar.' },
  };

  async function renderSources({ page }) {
    const o = await loadOverview();
    return [
      heading(page),
      h('article', { class: 'card panel grid' }, o.sources.map((x) => {
        const info = SOURCE_INFO[x.id] || {};
        return h('div', { class: 'list-row' },
          h('div', { class: 'list-main' },
            h('strong', null, x.name),
            h('small', null, info.gives || ''),
            x.detail ? h('small', { class: x.state === 'ok' ? '' : `t-${x.state}` }, x.detail) : null,
            x.state !== 'not_connected' && info.missing ? h('small', null, `Not served yet: ${info.missing}`) : null,
            x.last_ok || x.checked_at ? h('small', null,
              x.checked_at ? `Checked ${ago(x.checked_at)}` : '', x.last_ok ? ` · last answer ${ago(x.last_ok)}` : ' · no answer yet') : null),
          badge(x.state));
      })),
      h('ul', { class: 'notes' },
        h('li', null, 'Vista only shows what these sources report. It collects nothing itself, and holds no credentials that can change a project, deploy, restart or read a secret.'),
        h('li', null, 'When a source stops answering, Vista keeps its last answer, marks it stale, and shows live states as unknown.')),
    ];
  }

  // ---------- Start ----------
  function start() {
    buildNav();
    $('#range').value = state.range;
    $('#range').addEventListener('change', (e) => {
      state.range = e.target.value;
      store.set('vista.range', state.range);
      const { page, params } = parseHash();
      if (params.has('range') && (page.id === 'activity' || page.id === 'logs')) {
        params.set('range', state.range);
        params.delete('from'); params.delete('to');
        location.hash = `#/${page.id}?${params}`;
      } else {
        route();
      }
    });
    $('#refresh').addEventListener('click', () => route({ quiet: true }));
    $('#search').addEventListener('keydown', (e) => {
      if (e.key !== 'Enter') return;
      const q = e.target.value.trim();
      location.hash = hashFor('logs', { q, range: state.range === '1h' ? '24h' : state.range });
    });
    $('#drawerClose').addEventListener('click', () => closeDrawer());
    $('#drawerBackdrop').addEventListener('click', (e) => { if (e.target === e.currentTarget) closeDrawer(); });
    document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeDrawer(); });
    window.addEventListener('hashchange', () => route());
    setInterval(() => {
      if (document.visibilityState !== 'visible' || !$('#drawerBackdrop').hidden) return;
      if (parseHash().page.auto) route({ quiet: true });
    }, 30000);
    fetch('/healthz').then((r) => r.json()).then((b) => { $('#version').textContent = `Version ${b.version}`; }).catch(() => {});
    route();
  }
  start();
})();
