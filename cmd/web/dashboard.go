package web

import (
	"fmt"
	"net/http"
)

func DashboardHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Clawtivity Dashboard</title>
  <link href="/assets/css/output.css" rel="stylesheet" />
  <style>
    :root {
      --bg: #f4f6f8;
      --surface: #ffffff;
      --text: #17212b;
      --muted: #5c6b79;
      --line: #d9e0e7;
      --accent: #0b7285;
      --accent-soft: #d3f2f7;
      --warn: #e67700;
      --danger: #c92a2a;
      --ok: #2f9e44;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Helvetica, Arial, sans-serif;
      background: linear-gradient(160deg, #f9fbfc 0%, #eef4f8 100%);
      color: var(--text);
    }
    .container {
      max-width: 1200px;
      margin: 0 auto;
      padding: 16px;
    }
    .header {
      margin-bottom: 16px;
      padding: 16px;
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: 12px;
    }
    .header h1 { margin: 0 0 6px 0; font-size: 24px; }
    .header p { margin: 0; color: var(--muted); }
    .grid {
      display: grid;
      gap: 12px;
      grid-template-columns: 1fr;
    }
    .panel {
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 14px;
    }
    .filters {
      display: grid;
      gap: 10px;
      grid-template-columns: repeat(2, 1fr);
    }
    .filters .full { grid-column: span 2; }
    label { display: block; font-size: 12px; color: var(--muted); margin-bottom: 4px; }
    select, input, button {
      width: 100%;
      padding: 10px;
      border: 1px solid var(--line);
      border-radius: 8px;
      font-size: 14px;
      background: #fff;
    }
    button {
      background: var(--accent);
      color: #fff;
      border: 0;
      cursor: pointer;
      font-weight: 600;
    }
    .stats {
      display: grid;
      gap: 10px;
      grid-template-columns: repeat(2, 1fr);
    }
    .stat {
      border: 1px solid var(--line);
      border-radius: 10px;
      padding: 10px;
      background: #fbfdff;
    }
    .stat .label { color: var(--muted); font-size: 12px; }
    .stat .value { font-size: 22px; font-weight: 700; }
    .chart {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(74px, 1fr));
      gap: 8px;
      align-items: end;
      min-height: 170px;
      padding-top: 12px;
    }
    .bar-group { display: flex; gap: 2px; align-items: end; justify-content: center; }
    .bar {
      width: 14px;
      border-radius: 6px 6px 0 0;
      min-height: 2px;
    }
    .bar.in { background: var(--accent); }
    .bar.out { background: var(--warn); }
    .bar-label {
      margin-top: 5px;
      text-align: center;
      font-size: 11px;
      color: var(--muted);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .cost-list { display: grid; gap: 8px; }
    .cost-item {
      display: flex;
      justify-content: space-between;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 8px 10px;
      background: #fbfdff;
    }
    .timeline { display: grid; gap: 8px; }
    .timeline-item {
      border: 1px solid var(--line);
      border-left: 4px solid var(--accent);
      border-radius: 8px;
      padding: 10px;
      background: #fff;
    }
    .meta {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 6px;
      color: var(--muted);
      font-size: 12px;
    }
    .status-success { color: var(--ok); font-weight: 700; }
    .status-failed { color: var(--danger); font-weight: 700; }
    .status-pending, .status-in_progress { color: var(--warn); font-weight: 700; }
    @media (min-width: 900px) {
      .grid { grid-template-columns: 1.1fr 1fr; }
      .filters { grid-template-columns: repeat(5, 1fr); }
      .filters .full { grid-column: span 1; }
      .stats { grid-template-columns: repeat(4, 1fr); }
    }
  </style>
</head>
<body>
  <div class="container">
    <section class="header">
      <h1>Activity Dashboard</h1>
      <p>Local-first activity timeline and usage insights.</p>
    </section>

    <section class="panel">
      <div class="filters">
        <div>
          <label for="project-filter">Project</label>
          <select id="project-filter"><option value="">All projects</option></select>
        </div>
        <div>
          <label for="model-filter">Model</label>
          <select id="model-filter"><option value="">All models</option></select>
        </div>
        <div>
          <label for="date-from">From</label>
          <input id="date-from" type="date" />
        </div>
        <div>
          <label for="date-to">To</label>
          <input id="date-to" type="date" />
        </div>
        <div class="full">
          <label for="refresh-btn">Action</label>
          <button id="refresh-btn" type="button">Refresh</button>
        </div>
      </div>
    </section>

    <section class="panel">
      <div class="stats">
        <div class="stat"><div class="label">Activities</div><div class="value" id="stat-count">0</div></div>
        <div class="stat"><div class="label">Tokens In</div><div class="value" id="stat-in">0</div></div>
        <div class="stat"><div class="label">Tokens Out</div><div class="value" id="stat-out">0</div></div>
        <div class="stat"><div class="label">Cost Total</div><div class="value" id="stat-cost">$0.00</div></div>
      </div>
    </section>

    <section class="grid">
      <section class="panel">
        <h2>Token Usage</h2>
        <div id="token-chart" class="chart"></div>
      </section>

      <section class="panel">
        <h2>Cost by Project</h2>
        <div id="cost-by-project" class="cost-list"></div>
      </section>

      <section class="panel" style="grid-column: 1 / -1;">
        <h2>Recent Activity Timeline</h2>
        <div id="activity-timeline" class="timeline"></div>
      </section>
    </section>
  </div>

  <script>
    const state = {
      allActivities: [],
      filteredActivities: [],
      summary: { count: 0, tokens_in_total: 0, tokens_out_total: 0, cost_total: 0 }
    };

    const els = {
      project: document.getElementById('project-filter'),
      model: document.getElementById('model-filter'),
      from: document.getElementById('date-from'),
      to: document.getElementById('date-to'),
      refresh: document.getElementById('refresh-btn'),
      statCount: document.getElementById('stat-count'),
      statIn: document.getElementById('stat-in'),
      statOut: document.getElementById('stat-out'),
      statCost: document.getElementById('stat-cost'),
      timeline: document.getElementById('activity-timeline'),
      chart: document.getElementById('token-chart'),
      costByProject: document.getElementById('cost-by-project')
    };

    function formatNumber(value) {
      return new Intl.NumberFormat().format(value || 0);
    }

    function formatCurrency(value) {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency: 'USD', maximumFractionDigits: 4 }).format(value || 0);
    }

    function inDateRange(createdAt, from, to) {
      if (!from && !to) return true;
      const date = new Date(createdAt);
      if (from) {
        const fromDate = new Date(from + 'T00:00:00');
        if (date < fromDate) return false;
      }
      if (to) {
        const toDate = new Date(to + 'T23:59:59');
        if (date > toDate) return false;
      }
      return true;
    }

    async function fetchSummary(project, model) {
      const params = new URLSearchParams();
      if (project) params.set('project', project);
      if (model) params.set('model', model);
      const res = await fetch('/api/activity/summary?' + params.toString());
      if (!res.ok) throw new Error('summary fetch failed');
      return await res.json();
    }

    async function fetchActivities(project, model) {
      const params = new URLSearchParams();
      if (project) params.set('project', project);
      if (model) params.set('model', model);
      const res = await fetch('/api/activity?' + params.toString());
      if (!res.ok) throw new Error('activity fetch failed');
      return await res.json();
    }

    function updateSelectOptions(activities) {
      const projects = Array.from(new Set(activities.map(a => a.project_tag).filter(Boolean))).sort();
      const models = Array.from(new Set(activities.map(a => a.model).filter(Boolean))).sort();

      const selectedProject = els.project.value;
      const selectedModel = els.model.value;

      els.project.innerHTML = '<option value="">All projects</option>' + projects.map(v => '<option value="' + v + '">' + v + '</option>').join('');
      els.model.innerHTML = '<option value="">All models</option>' + models.map(v => '<option value="' + v + '">' + v + '</option>').join('');

      els.project.value = projects.includes(selectedProject) ? selectedProject : '';
      els.model.value = models.includes(selectedModel) ? selectedModel : '';
    }

    function applyLocalFilters() {
      const from = els.from.value;
      const to = els.to.value;
      state.filteredActivities = state.allActivities.filter(a => inDateRange(a.created_at, from, to));
    }

    function renderStats() {
      const inTotal = state.filteredActivities.reduce((sum, a) => sum + (a.tokens_in || 0), 0);
      const outTotal = state.filteredActivities.reduce((sum, a) => sum + (a.tokens_out || 0), 0);
      const costTotal = state.filteredActivities.reduce((sum, a) => sum + (a.cost_estimate || 0), 0);

      els.statCount.textContent = formatNumber(state.filteredActivities.length);
      els.statIn.textContent = formatNumber(inTotal);
      els.statOut.textContent = formatNumber(outTotal);
      els.statCost.textContent = formatCurrency(costTotal);
    }

    function renderTimeline() {
      const list = state.filteredActivities.slice(0, 30);
      if (list.length === 0) {
        els.timeline.innerHTML = '<div class="timeline-item">No activity matches current filters.</div>';
        return;
      }

      els.timeline.innerHTML = list.map(a => {
        const statusClass = 'status-' + (a.status || 'pending');
        const created = a.created_at ? new Date(a.created_at).toLocaleString() : 'unknown';
        return '<div class="timeline-item">'
          + '<div><strong>' + (a.project_tag || 'unknown-project') + '</strong> · ' + (a.model || 'unknown-model') + ' · <span class="' + statusClass + '">' + (a.status || 'pending') + '</span></div>'
          + '<div class="meta">'
          + '<span>session: ' + (a.session_key || '-') + '</span>'
          + '<span>tokens: ' + formatNumber(a.tokens_in || 0) + ' in / ' + formatNumber(a.tokens_out || 0) + ' out</span>'
          + '<span>cost: ' + formatCurrency(a.cost_estimate || 0) + '</span>'
          + '<span>channel: ' + (a.channel || '-') + '</span>'
          + '<span>category: ' + (a.category || '-') + '</span>'
          + '<span>created: ' + created + '</span>'
          + '</div></div>';
      }).join('');
    }

    function renderTokenChart() {
      const data = state.filteredActivities.slice(0, 20);
      if (data.length === 0) {
        els.chart.innerHTML = '<div>No token data.</div>';
        return;
      }

      const max = data.reduce((m, a) => Math.max(m, a.tokens_in || 0, a.tokens_out || 0), 1);

      els.chart.innerHTML = data.map(a => {
        const inHeight = Math.max(4, Math.round(((a.tokens_in || 0) / max) * 140));
        const outHeight = Math.max(4, Math.round(((a.tokens_out || 0) / max) * 140));
        const label = (a.project_tag || 'n/a').slice(0, 10);
        return '<div>'
          + '<div class="bar-group">'
          + '<div class="bar in" style="height:' + inHeight + 'px" title="tokens_in"></div>'
          + '<div class="bar out" style="height:' + outHeight + 'px" title="tokens_out"></div>'
          + '</div>'
          + '<div class="bar-label">' + label + '</div>'
          + '</div>';
      }).join('');
    }

    function renderCostByProject() {
      const totals = {};
      for (const item of state.filteredActivities) {
        const key = item.project_tag || 'unknown';
        totals[key] = (totals[key] || 0) + (item.cost_estimate || 0);
      }
      const rows = Object.entries(totals).sort((a, b) => b[1] - a[1]);
      if (rows.length === 0) {
        els.costByProject.innerHTML = '<div class="cost-item"><span>No project costs yet</span><span>$0.00</span></div>';
        return;
      }
      els.costByProject.innerHTML = rows.map(([project, cost]) => '<div class="cost-item"><span>' + project + '</span><strong>' + formatCurrency(cost) + '</strong></div>').join('');
    }

    function renderAll() {
      applyLocalFilters();
      renderStats();
      renderTimeline();
      renderTokenChart();
      renderCostByProject();
    }

    async function refreshData() {
      const project = els.project.value;
      const model = els.model.value;

      try {
        const [activities, summary] = await Promise.all([
          fetchActivities(project, model),
          fetchSummary(project, model)
        ]);

        state.allActivities = Array.isArray(activities) ? activities : [];
        state.summary = summary || {};

        updateSelectOptions(state.allActivities);
        renderAll();
      } catch (err) {
        els.timeline.innerHTML = '<div class="timeline-item">Failed to load activity data.</div>';
      }
    }

    els.refresh.addEventListener('click', refreshData);
    els.project.addEventListener('change', refreshData);
    els.model.addEventListener('change', refreshData);
    els.from.addEventListener('change', renderAll);
    els.to.addEventListener('change', renderAll);

    refreshData();
  </script>
</body>
</html>`)
}
