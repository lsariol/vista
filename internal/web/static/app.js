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
    token: 0,
  };

  // ---------- Pages and routing ----------
  const PAGES = [
    { id: 'overview', label: 'Overview', ico: '⌂', render: renderOverview, auto: true, sub: 'What needs attention on Spark and its projects' },
    { id: 'projects', label: 'Projects', ico: '◇', render: renderProjects, auto: true, sub: 'Every project Sonar watches, with its version and availability' },
    { id: 'spark', label: 'Spark', ico: 'ϟ', render: renderSpark, auto: true, sub: 'The server itself: its resources and the infrastructure every project relies on' },
    { id: 'activity', label: 'Activity', ico: '⌁', render: renderActivity, auto: true, sub: 'Deploys, error bursts and log gaps on one timeline' },
    { id: 'logs', label: 'Logs', ico: '▤', render: renderLogs, auto: false, sub: 'Search what every container wrote, kept by Blackbox' },
    { id: 'incidents', label: 'Incidents', ico: '⚠', render: renderIncidents, auto: true, sub: 'Availability and the days projects were down or degraded' },
    { id: 'sources', label: 'Sources', ico: '◉', render: renderSources, auto: true, sub: 'Where Vista\'s information comes from, and how fresh it is' },
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
      h('span', { class: 'nav-ico', 'aria-hidden': 'true' }, p.ico),
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
    $('#topTitle').textContent = page.label;
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

  function kpi(ico, title, big, small, extra) {
    return h('article', { class: 'card kpi' },
      h('div', { class: 'kpi-top' }, h('span', { class: 'ico', 'aria-hidden': 'true' }, ico), title),
      big, small ? h('div', { class: 'small' }, small) : null, extra || null);
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

  function sparkline(values) {
    const W = 100, H = 24;
    const pts = values.map((v, i) => (isNum(v) ? [i / Math.max(1, values.length - 1) * W, H - 2 - (Math.min(v, 100) / 100) * (H - 4)] : null));
    let d = '', pen = false;
    for (const p of pts) {
      if (!p) { pen = false; continue; }
      d += `${pen ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)} `;
      pen = true;
    }
    return s('svg', { class: 'spark-line', viewBox: `0 0 ${W} ${H}`, preserveAspectRatio: 'none', 'aria-hidden': 'true' },
      d ? s('path', { d, fill: 'none', stroke: '#27aafc', 'stroke-width': 1.5, 'vector-effect': 'non-scaling-stroke' }) : null);
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

  function drawChart(wrap, W, { height = 190, from, to, series, markers = [], label, unit = '%', yMax = 100 }) {
    const H = height, pad = { l: 40, r: 10, t: 10, b: 22 };
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
        s('text', { x: x0 - 6, y: y + 3, 'text-anchor': 'end' }, `${Math.round(f * yMax)}${unit}`));
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
        let v = p && isNum(p.v) ? fmtPct(p.v, 1) : 'no data';
        if (p && isNum(p.v) && se.band) v += ` (${fmtPct(p.min, 0)}–${fmtPct(p.max, 0)})`;
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
    const markers = host.deploys.map((d) => ({
      t: Date.parse(d.started),
      color: d.outcome === 'ok' ? '#20d58a' : '#fc526b',
      label: `${d.project} deploy ${d.outcome === 'ok' ? '' : `${d.outcome} `}${d.to || ''}`.replace(/\s+/g, ' ').trim(),
    }));
    return [
      lineChart({
        height: height || 190, band,
        from: host.from ? Date.parse(host.from) : undefined,
        to: host.to ? Date.parse(host.to) : undefined,
        label: 'Host CPU and memory use over time',
        series: [
          { name: 'CPU', color: '#30a7ff', points: pick('cpu_pct'), band },
          { name: 'Memory', color: '#ae75fc', points: pick('mem_used_pct'), band },
        ],
        markers,
      }),
      h('div', { class: 'legend' },
        h('span', null, h('i', { class: 'cpu' }), 'CPU'),
        h('span', null, h('i', { class: 'mem' }), 'Memory'),
        h('span', null, h('i', { class: 'deploy' }), 'Deploy'),
        h('span', null, h('i', { class: 'deploy-bad' }), 'Failed deploy')),
    ];
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
    const cpuVals = o.host.points.map((p) => (p.cpu_pct ? p.cpu_pct.avg : null));
    const memVals = o.host.points.map((p) => (p.mem_used_pct ? p.mem_used_pct.avg : null));
    const errs = o.recent_errors; // null when Blackbox can't be read
    const t = o.totals;

    const kpis = h('div', { class: 'grid grid-kpis' },
      kpi('ϟ', 'Spark health', h('div', { class: `big t-${o.spark.state}` }, '● ', LABEL[o.spark.state]),
        o.spark.reasons[0] || 'Host and core services answering'),
      kpi('◇', 'Projects', h('div', { class: 'big' }, `${t.healthy} / ${t.total}`),
        `healthy${t.degraded ? ` · ${t.degraded} degraded` : ''}${t.down ? ` · ${t.down} down` : ''}${t.unknown ? ` · ${t.unknown} unknown` : ''}${t.updating ? ` · ${t.updating} updating` : ''}`,
        progress(t)),
      kpi('▣', 'CPU', h('div', { class: 'big' }, o.host.cpu ? fmtPct(o.host.cpu.pct) : '—'),
        o.host.cpu ? `Average at ${fmtTime(o.host.cpu.at)}` : 'Not available', sparkline(cpuVals)),
      kpi('▥', 'Memory', h('div', { class: 'big' }, o.host.memory ? fmtPct(o.host.memory.pct) : '—'),
        o.host.memory ? `In use at ${fmtTime(o.host.memory.at)}` : 'Not available', sparkline(memVals)),
      kpi('⚠', 'Newest error', h('div', { class: `big ${errs && errs.length ? 'red' : ''}` }, errs ? (errs.length ? ago(errs[0].ts) : 'None') : '—'),
        errs ? (errs.length ? `From ${errs[0].project || errs[0].container}` : 'No error lines in 24 hours') : 'Blackbox not answering'),
      kpi('▤', 'Log storage', h('div', { class: 'big' }, o.logs ? fmtBytes(o.logs.stored_bytes) : '—'),
        o.logs ? `of ${fmtBytes(o.logs.max_bytes)} · oldest ${ago(o.logs.oldest)}` : 'Blackbox not answering',
        o.logs && o.logs.max_bytes ? storageBar(o.logs) : null),
    );

    return [
      heading(page, sourcesPill(o.sources)),
      kpis,
      h('div', { class: 'grid grid-2-1' }, attentionPanel(o.attention), h('article', { class: 'card panel' },
        h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '◇'), 'Project status'),
          h('a', { href: '#/projects' }, 'All projects')),
        donut(t))),
      h('div', { class: 'grid grid-2-1' },
        h('article', { class: 'card panel' },
          h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '〽'), 'Host CPU and memory'),
            h('span', { class: 'muted' }, rangeLabel(o.host.range))),
          hostChart(o.host)),
        deploysPanel(o.host.deploys, o.host.range)),
      projectsTable(o.projects, { title: 'Projects', compact: true }),
      recentErrorsPanel(errs),
      h('p', { class: 'note' }, 'States, versions, uptime and host measurements come from Sonar; errors and log storage from Blackbox. ',
        'Anything a source can\'t say is shown as unknown or “—”, never as zero or healthy.'),
    ];
  }

  function recentErrorsPanel(errs) {
    return h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '⚠'), 'Newest errors'),
        h('a', { href: hashFor('logs', { level: 'error', range: '24h' }) }, 'All errors in Logs')),
      !errs ? h('div', { class: 'empty' }, 'Blackbox can\'t be read.')
        : errs.length ? h('div', { class: 'logs' }, errs.map((r) => logRow(r, { compact: true })))
          : h('div', { class: 'empty' }, 'No error lines in the last 24 hours.'),
      h('p', { class: 'note' }, 'The newest error lines Blackbox holds. Error counts per project appear once Blackbox serves them.'));
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
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '♧'), 'Needs attention'),
        items.length > 12 ? h('span', { class: 'muted' }, `${items.length - 12} more`) : null),
      body);
  }

  function deploysPanel(deploys, range) {
    const list = deploys.slice().reverse().slice(0, 8);
    return h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '⇪'), 'Deploys'),
        h('a', { href: hashFor('activity', {}) }, 'Activity')),
      list.length ? list.map((d) => h('div', { class: 'list-row' },
        h('div', { class: 'list-main' },
          h('a', { href: `#/projects/${encodeURIComponent(d.project)}` }, d.project), ' ',
          h('span', { class: 'mono' }, d.to || 'unknown version'),
          h('small', null, d.from ? `from ${d.from} · ` : '', `took ${fmtSeconds(d.seconds)}`)),
        h('div', { class: 'attn-meta' }, badge(d.outcome === 'ok' ? 'healthy' : 'down', d.outcome === 'ok' ? 'OK' : d.outcome),
          h('div', { title: fmtFull(d.started) }, fmtTime(d.started)))))
        : h('div', { class: 'empty' }, `No deploys in the ${rangeLabel(range).toLowerCase()}.`));
  }

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
    h('td', null, dayStrip(p.days.slice(-30))));

    const head = h('tr', null, h('th', null, 'Project'), h('th', null, 'Status'), h('th', null, 'Version'),
      compact ? null : h('th', null, 'Monitoring'), h('th', { class: 'num' }, 'Response'), h('th', { class: 'num' }, 'Uptime 30d'),
      h('th', null, 'Last 30 days'));
    const cols = compact ? 6 : 7;
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
      title ? h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '◇'), title)) : null,
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
      section('Last deploy, 24 hours', h('div', { class: 'card' }, p.last_deploy
        ? [kv('Outcome', badge(p.last_deploy.outcome === 'ok' ? 'healthy' : 'down', p.last_deploy.outcome)),
          kv('Version', `${p.last_deploy.from || 'none'} → ${p.last_deploy.to || 'unknown'}`),
          kv('When', `${fmtFull(p.last_deploy.started)} (took ${fmtSeconds(p.last_deploy.seconds)})`)]
        : h('div', { class: 'muted' }, 'No deploy in the last 24 hours.'))),
      section('Errors, 24 hours', errorsHost),
      section('Resources', h('div', { class: 'card muted' }, 'Not available yet: Sonar doesn\'t serve per-project CPU, memory or network to Vista.')),
      section('Usage', h('div', { class: 'card muted' }, `Not instrumented: ${name} doesn't serve usage counts.`)),
      h('div', { class: 'log-actions' },
        h('a', { href: hashFor('logs', { project: name, range: '24h' }) }, 'Open its logs'),
        h('a', { href: hashFor('activity', { project: name }) }, 'Its activity')),
      h('p', { class: 'note' }, 'Read-only. Events close together are leads to investigate, not proof of cause.'),
    );
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
      h('div', { class: 'grid grid-3' },
        h('article', { class: 'card panel' },
          h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, 'ϟ'), 'Spark health')),
          h('div', { class: `big t-${o.spark.state}` }, '● ', LABEL[o.spark.state]),
          o.spark.reasons.length
            ? h('ul', { class: 'notes' }, o.spark.reasons.map((r) => h('li', null, r)))
            : null,
          h('div', { class: 'section' },
            kv('CPU now', o.host.cpu ? `${fmtPct(o.host.cpu.pct, 1)} at ${fmtTime(o.host.cpu.at)}` : '—'),
            kv('Memory now', o.host.memory ? `${fmtPct(o.host.memory.pct, 1)} at ${fmtTime(o.host.memory.at)}` : '—'))),
        infraCard(o.infrastructure),
        logsCard(o.logs)),
      h('article', { class: 'card panel grid' },
        h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '〽'), 'Host CPU and memory'),
          h('span', { class: 'muted' }, `${rangeLabel(o.host.range)} · average, with each bucket's range shaded`)),
        hostChart(o.host, { height: 260, band: true })),
      projectsTable(infraProjects, { title: 'Infrastructure projects' }),
      h('article', { class: 'card panel' },
        h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '○'), 'Not collected yet')),
        h('div', { class: 'bars' }, o.host.not_collected.map((n) => h('div', { class: 'list-row' }, h('span', null, n), badge('not_connected', 'Not available')))),
        h('p', { class: 'note' }, 'These appear here once Sonar collects and serves them to Vista.')),
    ];
  }

  function infraCard(infra) {
    return h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '◉'), 'Infrastructure')),
      infra.map((i) => h('div', { class: 'list-row' },
        h('div', { class: 'list-main' }, h('strong', null, i.name),
          h('small', null, i.reported ? (i.version ? `Version ${i.version}` : 'Version unknown') : 'Sonar doesn\'t report it yet')),
        i.reported ? badge(i.state) : badge('not_connected', 'Not reported'))),
      h('p', { class: 'note' }, 'As Sonar reports them. Vista runs no checks of its own.'));
  }

  function logsCard(l) {
    return h('article', { class: 'card panel' },
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '▤'), 'Log collection')),
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
    const kinds = { deploy: 'Deploy', deploy_failed: 'Failed deploy', log_gap: 'Log gap' };
    const projects = project && !a.projects.includes(project) ? [...a.projects, project] : a.projects;
    return [
      heading(page),
      h('div', { class: 'filters' },
        h('select', { class: 'select', 'aria-label': 'Project', onchange: (e) => nav({ project: e.target.value }) },
          h('option', { value: '' }, 'All projects'), projects.map((p) => h('option', { value: p, selected: p === project }, p))),
        h('select', { class: 'select', 'aria-label': 'Kind', onchange: (e) => nav({ kind: e.target.value }) },
          h('option', { value: '' }, 'Every kind'), Object.entries(kinds).map(([k, l]) => h('option', { value: k, selected: k === kind }, l))),
        h('span', { class: 'pill' }, rangeLabel(range))),
      sourceWarnings(a.sources),
      h('article', { class: 'card panel' },
        h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '⌁'), 'Timeline'),
          h('span', { class: 'muted' }, plural(events.length, 'event', 'events'))),
        events.length ? events.map((e) => h('div', { class: 'event' },
          h('div', { class: 'event-time', title: fmtFull(e.at) }, fmtTime(e.at)),
          h('div', null, h('span', { class: `kind ${e.kind}` }, kinds[e.kind] || e.kind)),
          h('div', null,
            h('div', { class: 'event-title' }, e.title),
            e.detail ? h('div', { class: 'event-detail' }, e.detail) : null,
            h('div', { class: 'log-actions' },
              e.link ? h('a', { href: e.link }, 'Logs around it') : null)),
          h('div', { class: 'event-src' }, e.source)))
          : h('div', { class: 'empty' }, 'Nothing happened in this range, as far as the connected sources know.')),
      h('ul', { class: 'notes' }, a.notes.map((n) => h('li', null, n))),
    ];
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
      h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '▤'), 'Lines'), summary),
      results, footer,
      h('p', { class: 'note' }, 'Select a line for its fields and the lines around it. Times are your local time.')));
    return out;
  }

  async function contextPanel(id) {
    try {
      const c = await api(`/api/v1/logs/${encodeURIComponent(id)}/context?before=30&after=30`);
      return h('article', { class: 'card panel grid' },
        h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '⌖'), 'Line in context'),
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
      h('div', { class: 'grid grid-1-1' },
        h('article', { class: 'card panel' },
          h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '⚠'), 'Days with trouble'),
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
          h('div', { class: 'panel-head' }, h('div', { class: 'panel-title' }, h('span', { class: 'ico' }, '◇'), 'Availability')),
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
    sonar: { gives: 'Project states, versions, uptime and daily history; host CPU and memory; deploys.',
      missing: 'Spark\'s own conditions, the projects the public status leaves out, per-project resources, disk, network, load, restarts, and when each state change began.' },
    blackbox: { gives: 'Log search, the lines around a line, the newest errors, log sources, gaps and collection health.',
      missing: 'Error counts per project and hour.' },
    lighthouse: { gives: 'Deploy records, rollbacks and the version each project runs.',
      missing: 'A read-only API on the spark network. Until then, deploys come from Sonar.' },
    apps: { gives: 'Usage counts each application defines, such as games played or commands run.',
      missing: 'No application serves them yet.' },
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
