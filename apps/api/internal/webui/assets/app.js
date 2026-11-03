const POLL_INTERVAL_MS = 30_000;
const STATUS_ORDER = { DOWN: 0, DEGRADED: 1, UNKNOWN: 2, UP: 3 };
const STATUS_LABELS = { UP: "Opérationnel", DOWN: "En panne", DEGRADED: "Dégradé", UNKNOWN: "Inconnu" };
const PERIOD_LABELS = { 1: "1 heure", 6: "6 heures", 24: "24 heures", 168: "7 jours", 720: "30 jours" };

const state = {
  applications: null,
  detail: null,
  incidents: [],
  settings: null,
  loading: true,
  refreshing: false,
  error: null,
  search: "",
  status: "ALL",
  sort: "severity",
  periodHours: 24,
  lastSync: null,
};

const appRoot = document.querySelector("#app");
const refreshButton = document.querySelector('[data-action="refresh"]');
const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "fuseau local";
document.querySelector("#local-timezone").textContent = timezone;
document.querySelector("#timezone-sidebar").textContent = timezone;

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function number(value, digits = 0) {
  return new Intl.NumberFormat("fr-FR", { maximumFractionDigits: digits }).format(Number(value) || 0);
}

function formatPercent(value) {
  return `${number(value, value >= 99 ? 2 : 1)} %`;
}

function formatLatency(value) {
  return `${number(value)} ms`;
}

function formatDate(value, options = {}) {
  if (!value) return "Jamais";
  const formatOptions = options.compact
    ? { hour: "2-digit", minute: "2-digit", timeZone: timezone }
    : { dateStyle: "medium", timeStyle: "medium", timeZone: timezone };
  return new Intl.DateTimeFormat("fr-FR", formatOptions).format(new Date(value));
}

function relativeTime(value) {
  if (!value) return "Aucun contrôle";
  const deltaSeconds = Math.round((new Date(value).getTime() - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat("fr", { numeric: "auto" });
  if (Math.abs(deltaSeconds) < 60) return formatter.format(deltaSeconds, "second");
  const minutes = Math.round(deltaSeconds / 60);
  if (Math.abs(minutes) < 60) return formatter.format(minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(hours, "hour");
  return formatter.format(Math.round(hours / 24), "day");
}

function statusClass(status) {
  return `status-${String(status || "UNKNOWN").toLowerCase()}`;
}

function statusPill(status) {
  const safeStatus = STATUS_LABELS[status] ? status : "UNKNOWN";
  return `<span class="status-pill ${statusClass(safeStatus)}">${STATUS_LABELS[safeStatus]}</span>`;
}

function route() {
  const match = window.location.hash.match(/^#\/applications\/([0-9a-f-]+)$/i);
  return match ? { name: "detail", id: match[1] } : { name: "overview" };
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: {
      Accept: "application/json",
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(options.headers || {}),
    },
  });
  if (!response.ok) {
    let message = `La requête a échoué (${response.status})`;
    try {
      const payload = await response.json();
      if (payload.error) message = payload.error;
    } catch {
      // Keep the status-based fallback when the body is not JSON.
    }
    throw new Error(message);
  }
  return response.status === 204 ? null : response.json();
}

function setRefreshing(active) {
  state.refreshing = active;
  refreshButton.classList.toggle("is-loading", active);
  refreshButton.disabled = active;
}

async function loadOverview({ silent = false } = {}) {
  if (!silent) {
    state.loading = true;
    state.error = null;
    render();
  }
  setRefreshing(true);
  try {
    [state.applications, state.incidents] = await Promise.all([
      api("/dashboard"),
      api("/incidents?limit=50"),
    ]);
    state.error = null;
    state.lastSync = new Date();
  } catch (error) {
    state.error = error;
    if (silent) showToast("Impossible d’actualiser la vue globale.");
  } finally {
    state.loading = false;
    setRefreshing(false);
    render();
  }
}

async function loadDetail(applicationID, { silent = false } = {}) {
  if (!silent) {
    state.loading = true;
    state.error = null;
    state.detail = null;
    render();
  }
  setRefreshing(true);
  try {
    [state.detail, state.incidents] = await Promise.all([
      api(`/dashboard/applications/${encodeURIComponent(applicationID)}?period_hours=${state.periodHours}`),
      api(`/incidents?application_id=${encodeURIComponent(applicationID)}&limit=100`),
    ]);
    state.error = null;
    state.lastSync = new Date();
  } catch (error) {
    state.error = error;
    if (silent) showToast("Impossible d’actualiser la fiche application.");
  } finally {
    state.loading = false;
    setRefreshing(false);
    render();
  }
}

async function refreshCurrent(options = {}) {
  const current = route();
  if (current.name === "detail") return loadDetail(current.id, options);
  return loadOverview(options);
}

function showToast(message) {
  const region = document.querySelector("#toast-region");
  const toast = document.createElement("div");
  toast.className = "toast";
  toast.textContent = message;
  region.append(toast);
  window.setTimeout(() => toast.remove(), 4500);
}

function loadingTemplate() {
  return `
    <section aria-label="Chargement du tableau de bord">
      <div class="page-heading">
        <div><div class="skeleton skeleton-line" style="width:110px"></div><div class="skeleton skeleton-line" style="width:310px;height:38px;margin-top:14px"></div></div>
      </div>
      <div class="summary-grid">${Array.from({ length: 4 }, () => '<div class="skeleton skeleton-card"></div>').join("")}</div>
      <div class="panel">${Array.from({ length: 5 }, () => '<div class="app-row"><div class="skeleton skeleton-line" style="width:65%"></div><div class="skeleton skeleton-line"></div><div class="skeleton skeleton-line"></div><div class="skeleton skeleton-line"></div></div>').join("")}</div>
    </section>`;
}

function errorTemplate() {
  return `
    <section class="state-view">
      <div class="state-content">
        <div class="state-icon" aria-hidden="true">!</div>
        <h2>Le tableau de bord est indisponible</h2>
        <p>${escapeHTML(state.error?.message || "Une erreur inattendue s’est produite.")}</p>
        <button class="button" type="button" data-action="retry">Réessayer</button>
      </div>
    </section>`;
}

function summaryCards(applications) {
  const activeIncidents = state.incidents.filter((item) => item.status !== "RESOLVED");
  const down = applications.filter((item) => item.status === "DOWN").length;
  const healthy = applications.filter((item) => item.status === "UP").length;
  const measured = applications.filter((item) => item.total_checks_24h > 0);
  const uptime = measured.length ? measured.reduce((sum, item) => sum + item.uptime_24h, 0) / measured.length : 0;
  const latency = measured.length ? measured.reduce((sum, item) => sum + item.average_response_time_ms, 0) / measured.length : 0;

  return `
    <div class="summary-grid" aria-label="Synthèse des dernières 24 heures">
      <article class="metric-card ${down ? "danger" : "success"}">
        <span class="metric-label"><i></i> Incidents actifs</span>
        <strong class="metric-value">${activeIncidents.length}</strong>
        <span class="metric-detail">${activeIncidents.length ? `${activeIncidents.length} incident${activeIncidents.length > 1 ? "s" : ""} à examiner` : "Aucun incident actif"}</span>
      </article>
      <article class="metric-card success">
        <span class="metric-label"><i></i> Applications stables</span>
        <strong class="metric-value">${healthy}<small> / ${applications.length}</small></strong>
        <span class="metric-detail">État courant des moniteurs</span>
      </article>
      <article class="metric-card">
        <span class="metric-label"><i></i> Disponibilité moyenne</span>
        <strong class="metric-value">${formatPercent(uptime)}</strong>
        <span class="metric-detail">Sur les dernières 24 heures</span>
      </article>
      <article class="metric-card">
        <span class="metric-label"><i></i> Latence moyenne</span>
        <strong class="metric-value">${formatLatency(latency)}</strong>
        <span class="metric-detail">Réponses réussies uniquement</span>
      </article>
    </div>`;
}

function filteredApplications() {
  let items = [...(state.applications || [])];
  const query = state.search.trim().toLocaleLowerCase("fr");
  if (query) items = items.filter((item) => `${item.name} ${item.owner}`.toLocaleLowerCase("fr").includes(query));
  if (state.status !== "ALL") items = items.filter((item) => item.status === state.status);

  items.sort((left, right) => {
    if (state.sort === "name") return left.name.localeCompare(right.name, "fr");
    if (state.sort === "uptime") return right.uptime_24h - left.uptime_24h;
    if (state.sort === "latency") return right.average_response_time_ms - left.average_response_time_ms;
    return (STATUS_ORDER[left.status] ?? 9) - (STATUS_ORDER[right.status] ?? 9) || left.name.localeCompare(right.name, "fr");
  });
  return items;
}

function applicationRows(items) {
  if (!items.length) return '<div class="empty-row">Aucune application ne correspond aux filtres.</div>';
  return items.map((item) => `
    <a class="app-row" href="#/applications/${escapeHTML(item.id)}" data-app-id="${escapeHTML(item.id)}">
      <div class="app-name"><strong>${escapeHTML(item.name)}</strong><small>${escapeHTML(item.owner)} · ${item.monitors} moniteur${item.monitors > 1 ? "s" : ""}</small></div>
      ${statusPill(item.status)}
      <div class="data-value"><strong>${formatPercent(item.uptime_24h)}</strong><div class="progress"><span style="width:${clamp(item.uptime_24h, 0, 100)}%"></span></div></div>
      <div class="data-value"><strong>${formatLatency(item.average_response_time_ms)}</strong></div>
      <div class="data-value" title="${escapeHTML(formatDate(item.last_checked_at))}">${relativeTime(item.last_checked_at)}</div>
      <span class="row-arrow" aria-hidden="true">›</span>
    </a>`).join("");
}

function overviewTemplate() {
  const applications = state.applications || [];
  const visible = filteredApplications();
  return `
    <section>
      <header class="page-heading">
        <div>
          <p class="eyebrow">Situation en temps réel</p>
          <h1>Vue d’ensemble</h1>
          <p class="subtitle">Repérez les interruptions, les ralentissements et les services sans signal avant qu’ils ne deviennent invisibles.</p>
        </div>
        <span class="last-sync">Synchronisé ${state.lastSync ? relativeTime(state.lastSync) : "—"}</span>
      </header>
      ${summaryCards(applications)}
      <div class="toolbar" aria-label="Filtres des applications">
        <label class="search-box">
          <svg aria-hidden="true" viewBox="0 0 24 24"><circle cx="11" cy="11" r="7"></circle><path d="m20 20-4-4"></path></svg>
          <span class="sr-only"></span>
          <input type="search" data-control="search" value="${escapeHTML(state.search)}" placeholder="Rechercher une application ou une équipe…" aria-label="Rechercher" />
        </label>
        <div class="filter-chips">
          ${["ALL", "DOWN", "DEGRADED", "UP", "UNKNOWN"].map((status) => `<button class="chip ${state.status === status ? "is-active" : ""}" type="button" data-status="${status}">${status === "ALL" ? "Toutes" : STATUS_LABELS[status]}</button>`).join("")}
        </div>
        <label class="select-wrap">
          <select class="select-control" data-control="sort" aria-label="Trier les applications">
            <option value="severity" ${state.sort === "severity" ? "selected" : ""}>Priorité opérationnelle</option>
            <option value="name" ${state.sort === "name" ? "selected" : ""}>Nom</option>
            <option value="uptime" ${state.sort === "uptime" ? "selected" : ""}>Disponibilité</option>
            <option value="latency" ${state.sort === "latency" ? "selected" : ""}>Latence</option>
          </select>
        </label>
      </div>
      <div class="panel" aria-label="${visible.length} applications affichées">
        <div class="app-list-header" aria-hidden="true"><span>Application</span><span>État</span><span>Disponibilité</span><span>Latence</span><span>Dernier contrôle</span><span></span></div>
        <div id="application-rows">${applicationRows(visible)}</div>
      </div>
      <div class="section-heading"><h2>Incidents récents</h2><span>${state.incidents.length} événement${state.incidents.length > 1 ? "s" : ""}</span></div>
      <div class="panel incident-list">${incidentRows(state.incidents.slice(0, 8))}</div>
    </section>`;
}

function latencyChart(points) {
  if (!points.length) return '<div class="state-view" style="min-height:210px"><div class="state-content"><p>Aucune mesure sur cette période.</p></div></div>';
  const width = 640;
  const height = 190;
  const padding = { top: 14, right: 8, bottom: 24, left: 42 };
  const values = points.map((point) => Number(point.average_response_time_ms) || 0);
  const max = Math.max(...values, 10);
  const coords = values.map((value, index) => {
    const x = padding.left + (index / Math.max(values.length - 1, 1)) * (width - padding.left - padding.right);
    const y = padding.top + (1 - value / max) * (height - padding.top - padding.bottom);
    return [x, y];
  });
  const line = coords.map(([x, y], index) => `${index ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  const area = `${line} L${coords.at(-1)[0].toFixed(1)},${height - padding.bottom} L${coords[0][0].toFixed(1)},${height - padding.bottom} Z`;
  const grid = [0, .25, .5, .75, 1].map((ratio) => {
    const y = padding.top + ratio * (height - padding.top - padding.bottom);
    const label = Math.round(max * (1 - ratio));
    return `<line class="chart-grid-line" x1="${padding.left}" x2="${width - padding.right}" y1="${y}" y2="${y}"/><text class="chart-label" x="0" y="${y + 3}">${label} ms</text>`;
  }).join("");
  return `<svg viewBox="0 0 ${width} ${height}" role="img" aria-label="Courbe de latence moyenne">
    <defs><linearGradient id="latency-gradient" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#b9f26c" stop-opacity=".2"/><stop offset="1" stop-color="#b9f26c" stop-opacity="0"/></linearGradient></defs>
    ${grid}<path class="chart-area" d="${area}"/><path class="chart-line" d="${line}"/>
    <circle class="chart-dot" cx="${coords.at(-1)[0]}" cy="${coords.at(-1)[1]}" r="3.5"/>
  </svg><div class="chart-axis"><span>${formatDate(points[0].timestamp, { compact: true })}</span><span>${formatDate(points.at(-1).timestamp, { compact: true })}</span></div>`;
}

function availabilityChart(points) {
  if (!points.length) return '<div class="state-view" style="min-height:210px"><div class="state-content"><p>Aucune mesure sur cette période.</p></div></div>';
  return `<div class="availability-bars" role="img" aria-label="Disponibilité par intervalle">
    ${points.map((point) => {
      const uptime = clamp(Number(point.uptime) || 0, 0, 100);
      const tone = uptime < 80 ? "is-down" : uptime < 99 ? "is-warning" : "";
      return `<div class="availability-bar ${tone}" title="${escapeHTML(formatDate(point.timestamp))} · ${formatPercent(uptime)}"><span style="height:${Math.max(uptime, 3)}%"></span></div>`;
    }).join("")}
  </div><div class="chart-axis"><span>${formatDate(points[0].timestamp, { compact: true })}</span><span>${formatDate(points.at(-1).timestamp, { compact: true })}</span></div>`;
}

function monitorRows(monitors) {
  if (!monitors.length) return '<div class="empty-row">Aucun moniteur actif dans cet environnement.</div>';
  return monitors.map((monitor) => `
    <div class="monitor-row">
      <div class="monitor-target"><strong>${escapeHTML(monitor.name)}</strong><small>${escapeHTML(monitor.method)} · ${escapeHTML(monitor.url)}</small></div>
      ${statusPill(monitor.status)}
      <span class="data-value"><strong>${formatPercent(monitor.uptime)}</strong></span>
      <span class="data-value"><strong>${formatLatency(monitor.average_response_time_ms)}</strong></span>
      <span class="data-value" title="${escapeHTML(formatDate(monitor.last_checked_at))}">${relativeTime(monitor.last_checked_at)}</span>
      <button class="config-link" type="button" data-config-monitor="${escapeHTML(monitor.id)}">Alertes</button>
    </div>`).join("");
}

function environmentCards(environments) {
  if (!environments.length) return '<div class="panel empty-row">Aucun environnement configuré.</div>';
  return environments.map((environment) => `
    <article class="panel environment-card">
      <header class="environment-head">
        <div class="environment-name"><h3>${escapeHTML(environment.name)}</h3><small>${escapeHTML(environment.base_url)}</small></div>
        ${environment.enabled ? statusPill(environment.status) : '<span class="status-pill status-unknown">Désactivé</span>'}
      </header>
      <div>${monitorRows(environment.monitors || [])}</div>
    </article>`).join("");
}

function eventRows(events) {
  if (!events.length) return '<div class="empty-row">Aucun contrôle récent.</div>';
  return events.map((event) => `
    <div class="event-row">
      <time class="event-time" datetime="${escapeHTML(event.checked_at)}" title="${escapeHTML(formatDate(event.checked_at))}">${relativeTime(event.checked_at)}</time>
      <div class="event-name"><strong>${escapeHTML(event.monitor_name)}</strong><small>${escapeHTML(event.environment_name)}${event.error_message ? ` · <span class="error-message">${escapeHTML(event.error_message)}</span>` : ""}</small></div>
      ${statusPill(event.computed_status)}
      <span class="data-value">HTTP ${event.status_code || "—"}</span>
      <span class="data-value">${formatLatency(event.response_time_ms)}</span>
    </div>`).join("");
}

function incidentRows(incidents) {
  if (!incidents.length) return '<div class="empty-row">Aucun incident enregistré.</div>';
  return incidents.map((incident) => `
    <div class="incident-row">
      <span class="incident-kind ${incident.kind === "LATENCY" ? "latency" : "availability"}">${incident.kind === "LATENCY" ? "Latence" : "Disponibilité"}</span>
      <div class="event-name"><strong>${escapeHTML(incident.summary)}</strong><small>${escapeHTML(incident.application_name)} · ${escapeHTML(incident.environment_name)} · ${escapeHTML(incident.monitor_name)}</small></div>
      <span class="status-pill ${incident.status === "RESOLVED" ? "status-up" : incident.status === "ACKNOWLEDGED" ? "status-degraded" : "status-down"}">${incident.status === "RESOLVED" ? "Résolu" : incident.status === "ACKNOWLEDGED" ? "Acquitté" : "Ouvert"}</span>
      <time class="event-time" datetime="${escapeHTML(incident.started_at)}" title="${escapeHTML(formatDate(incident.started_at))}">${relativeTime(incident.started_at)}</time>
      ${incident.status === "OPEN" ? `<button class="config-link" type="button" data-ack-incident="${escapeHTML(incident.id)}">Acquitter</button>` : "<span></span>"}
    </div>`).join("");
}

function localDateTimeInput(value) {
  const date = value ? new Date(value) : new Date();
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
}

function findMonitor(monitorID) {
  return state.detail?.environments?.flatMap((environment) => environment.monitors || []).find((monitor) => monitor.id === monitorID);
}

async function openMonitorSettings(monitorID) {
  const monitor = findMonitor(monitorID);
  if (!monitor) return;
  state.settings = { monitor, loading: true, rule: null, maintenances: [] };
  renderSettings();
  try {
    const [rule, maintenances] = await Promise.all([
      api(`/monitors/${encodeURIComponent(monitorID)}/alert-rule`),
      api(`/monitors/${encodeURIComponent(monitorID)}/maintenance-windows`),
    ]);
    state.settings = { monitor, loading: false, rule, maintenances };
    renderSettings();
  } catch (error) {
    state.settings = null;
    renderSettings();
    showToast(`Configuration indisponible : ${error.message}`);
  }
}

function closeSettings() {
  state.settings = null;
  renderSettings();
}

function renderSettings() {
  const root = document.querySelector("#modal-root");
  if (!state.settings) {
    root.innerHTML = "";
    return;
  }
  const { monitor, loading, rule, maintenances } = state.settings;
  if (loading) {
    root.innerHTML = '<div class="modal-backdrop"><section class="settings-modal"><div class="state-view" style="min-height:240px"><div class="state-content"><p>Chargement de la configuration…</p></div></div></section></div>';
    return;
  }
  const startsAt = new Date(Date.now() + 5 * 60_000);
  const endsAt = new Date(startsAt.getTime() + 60 * 60_000);
  root.innerHTML = `
    <div class="modal-backdrop" data-action="close-settings">
      <section class="settings-modal" role="dialog" aria-modal="true" aria-labelledby="settings-title" data-modal-panel>
        <header class="settings-head"><div><p class="eyebrow">Configuration</p><h2 id="settings-title">${escapeHTML(monitor.name)}</h2><p>${escapeHTML(monitor.url)}</p></div><button class="modal-close" type="button" data-action="close-settings" aria-label="Fermer">×</button></header>
        <div class="settings-body">
          <form data-form="alert-rule" data-monitor-id="${escapeHTML(monitor.id)}">
            <div class="form-section-title"><div><h3>Règle d’alerte</h3><p>Un webhook unique est envoyé à l’ouverture puis, si activé, au rétablissement.</p></div><label class="switch"><input name="enabled" type="checkbox" ${rule.enabled ? "checked" : ""}/><span></span><strong>Active</strong></label></div>
            <label class="field field-wide"><span>URL du webhook</span><input name="webhook_url" type="url" value="${escapeHTML(rule.webhook_url)}" placeholder="https://hooks.example.com/go-maso" /></label>
            <div class="form-grid">
              <label class="field"><span>Échecs consécutifs</span><input name="failure_threshold" type="number" min="1" max="20" value="${rule.failure_threshold}" required /></label>
              <label class="field"><span>Fenêtre d’observation (s)</span><input name="failure_window_seconds" type="number" min="30" max="86400" value="${rule.failure_window_seconds}" required /></label>
              <label class="field"><span>Seuil de latence (ms)</span><input name="latency_threshold_ms" type="number" min="0" max="600000" value="${rule.latency_threshold_ms}" /><small>0 désactive ce signal</small></label>
              <label class="field"><span>Mesures lentes consécutives</span><input name="latency_consecutive_checks" type="number" min="1" max="20" value="${rule.latency_consecutive_checks}" required /></label>
              <label class="field"><span>Cooldown (s)</span><input name="cooldown_seconds" type="number" min="0" max="86400" value="${rule.cooldown_seconds}" required /></label>
              <label class="check-field"><input name="notify_recovery" type="checkbox" ${rule.notify_recovery ? "checked" : ""}/><span>Notifier le rétablissement</span></label>
            </div>
            <div class="form-actions"><button class="button" type="submit">Enregistrer la règle</button></div>
          </form>
          <div class="settings-divider"></div>
          <form data-form="maintenance" data-monitor-id="${escapeHTML(monitor.id)}">
            <div class="form-section-title"><div><h3>Fenêtre de maintenance</h3><p>Les nouveaux incidents et notifications sont suspendus pendant la période.</p></div></div>
            <div class="form-grid maintenance-grid">
              <label class="field field-wide"><span>Motif</span><input name="reason" maxlength="240" placeholder="Déploiement planifié" required /></label>
              <label class="field"><span>Début · ${escapeHTML(timezone)}</span><input name="starts_at" type="datetime-local" value="${localDateTimeInput(startsAt)}" required /></label>
              <label class="field"><span>Fin · ${escapeHTML(timezone)}</span><input name="ends_at" type="datetime-local" value="${localDateTimeInput(endsAt)}" required /></label>
            </div>
            <div class="form-actions"><button class="secondary-button" type="submit">Planifier</button></div>
          </form>
          <div class="maintenance-list">
            ${maintenances.length ? maintenances.map((window) => `<div class="maintenance-item"><div><strong>${escapeHTML(window.reason)}</strong><small>${formatDate(window.starts_at)} → ${formatDate(window.ends_at)}</small></div><button class="config-link" type="button" data-cancel-maintenance="${escapeHTML(window.id)}">Annuler</button></div>`).join("") : '<p class="muted-copy">Aucune maintenance planifiée.</p>'}
          </div>
        </div>
      </section>
    </div>`;
}

function detailTemplate() {
  const detail = state.detail;
  if (!detail) return errorTemplate();
  const latestLatency = detail.timeline?.length ? detail.timeline.at(-1).average_response_time_ms : detail.average_response_time_ms;
  return `
    <section>
      <nav class="breadcrumb" aria-label="Fil d’Ariane"><a href="#/">Vue d’ensemble</a><span>/</span><strong>${escapeHTML(detail.name)}</strong></nav>
      <header class="page-heading detail-heading">
        <div>
          <p class="eyebrow">Fiche application</p>
          <div class="detail-title-line"><h1>${escapeHTML(detail.name)}</h1>${statusPill(detail.status)}</div>
          <p class="subtitle">Responsable : ${escapeHTML(detail.owner)} · ${detail.monitors} moniteur${detail.monitors > 1 ? "s" : ""} · Données générées à ${formatDate(detail.generated_at)}</p>
        </div>
        <div class="period-picker" aria-label="Période d’analyse">
          ${[[1,"1 h"],[6,"6 h"],[24,"24 h"],[168,"7 j"],[720,"30 j"]].map(([hours, label]) => `<button class="period-button ${state.periodHours === hours ? "is-active" : ""}" type="button" data-period="${hours}">${label}</button>`).join("")}
        </div>
      </header>

      <div class="summary-grid">
        <article class="metric-card ${detail.status === "DOWN" ? "danger" : ""}"><span class="metric-label"><i></i> État courant</span><strong class="metric-value" style="font-size:24px">${STATUS_LABELS[detail.status] || "Inconnu"}</strong><span class="metric-detail">Dernier contrôle ${relativeTime(detail.last_checked_at)}</span></article>
        <article class="metric-card"><span class="metric-label"><i></i> Disponibilité</span><strong class="metric-value">${formatPercent(detail.period_summary.uptime)}</strong><span class="metric-detail">Sur ${PERIOD_LABELS[state.periodHours]}</span></article>
        <article class="metric-card"><span class="metric-label"><i></i> Latence moyenne</span><strong class="metric-value">${formatLatency(detail.period_summary.average_response_time_ms)}</strong><span class="metric-detail">${number(detail.period_summary.total_checks)} contrôles analysés</span></article>
        <article class="metric-card"><span class="metric-label"><i></i> Dernière mesure</span><strong class="metric-value">${formatLatency(detail.last_response_time_ms)}</strong><span class="metric-detail">HTTP ${detail.last_status_code || "—"}</span></article>
      </div>

      <div class="chart-grid">
        <article class="panel chart-card">
          <header class="panel-heading"><div><h2>Temps de réponse</h2><p>Moyenne des réponses réussies par intervalle</p></div><strong class="chart-value">${formatLatency(latestLatency)}</strong></header>
          <div class="chart">${latencyChart(detail.timeline || [])}</div>
        </article>
        <article class="panel chart-card">
          <header class="panel-heading"><div><h2>Disponibilité</h2><p>Part des contrôles réussis par intervalle</p></div><strong class="chart-value">${formatPercent(detail.period_summary.uptime)}</strong></header>
          <div class="chart">${availabilityChart(detail.timeline || [])}</div>
        </article>
      </div>

      <div class="section-heading"><h2>Incidents</h2><span>${state.incidents.filter((item) => item.status !== "RESOLVED").length} actif${state.incidents.filter((item) => item.status !== "RESOLVED").length > 1 ? "s" : ""}</span></div>
      <div class="panel incident-list">${incidentRows(state.incidents)}</div>

      <div class="section-heading"><h2>Environnements & moniteurs</h2><span>${detail.environments?.length || 0} environnement${detail.environments?.length > 1 ? "s" : ""}</span></div>
      ${environmentCards(detail.environments || [])}

      <div class="section-heading"><h2>Événements récents</h2><span>30 derniers contrôles · heures en ${escapeHTML(timezone)}</span></div>
      <div class="panel event-list">${eventRows(detail.recent_events || [])}</div>
    </section>`;
}

function renderRowsOnly() {
  const container = document.querySelector("#application-rows");
  if (container) container.innerHTML = applicationRows(filteredApplications());
}

function render() {
  if (state.loading) {
    appRoot.innerHTML = loadingTemplate();
    return;
  }
  if (state.error && !(state.applications || state.detail)) {
    appRoot.innerHTML = errorTemplate();
    return;
  }
  appRoot.innerHTML = route().name === "detail" ? detailTemplate() : overviewTemplate();
}

document.addEventListener("click", (event) => {
  const action = event.target.closest("[data-action]")?.dataset.action;
  if (action === "refresh" || action === "retry") refreshCurrent();
  if (action === "home") window.location.hash = "#/";
  if (action === "close-settings" && !event.target.closest("[data-modal-panel]")) closeSettings();
  if (event.target.closest('.modal-close[data-action="close-settings"]')) closeSettings();

  const monitorID = event.target.closest("[data-config-monitor]")?.dataset.configMonitor;
  if (monitorID) openMonitorSettings(monitorID);

  const incidentID = event.target.closest("[data-ack-incident]")?.dataset.ackIncident;
  if (incidentID) acknowledgeIncident(incidentID);

  const maintenanceID = event.target.closest("[data-cancel-maintenance]")?.dataset.cancelMaintenance;
  if (maintenanceID) cancelMaintenance(maintenanceID);

  const status = event.target.closest("[data-status]")?.dataset.status;
  if (status) {
    state.status = status;
    render();
  }

  const period = Number(event.target.closest("[data-period]")?.dataset.period);
  if (period && period !== state.periodHours) {
    state.periodHours = period;
    const current = route();
    if (current.name === "detail") loadDetail(current.id);
  }
});

document.addEventListener("submit", async (event) => {
  const form = event.target.closest("[data-form]");
  if (!form) return;
  event.preventDefault();
  const submit = form.querySelector('[type="submit"]');
  submit.disabled = true;
  try {
    const data = new FormData(form);
    const monitorID = form.dataset.monitorId;
    if (form.dataset.form === "alert-rule") {
      const payload = {
        enabled: data.has("enabled"),
        webhook_url: String(data.get("webhook_url") || ""),
        failure_threshold: Number(data.get("failure_threshold")),
        failure_window_seconds: Number(data.get("failure_window_seconds")),
        latency_threshold_ms: Number(data.get("latency_threshold_ms")),
        latency_consecutive_checks: Number(data.get("latency_consecutive_checks")),
        cooldown_seconds: Number(data.get("cooldown_seconds")),
        notify_recovery: data.has("notify_recovery"),
      };
      const rule = await api(`/monitors/${encodeURIComponent(monitorID)}/alert-rule`, { method: "PUT", body: JSON.stringify(payload) });
      state.settings.rule = rule;
      showToast("Règle d’alerte enregistrée.");
    } else {
      await api(`/monitors/${encodeURIComponent(monitorID)}/maintenance-windows`, {
        method: "POST",
        body: JSON.stringify({
          reason: String(data.get("reason")),
          starts_at: new Date(String(data.get("starts_at"))).toISOString(),
          ends_at: new Date(String(data.get("ends_at"))).toISOString(),
        }),
      });
      state.settings.maintenances = await api(`/monitors/${encodeURIComponent(monitorID)}/maintenance-windows`);
      showToast("Maintenance planifiée.");
    }
    renderSettings();
  } catch (error) {
    showToast(error.message);
    submit.disabled = false;
  }
});

async function acknowledgeIncident(incidentID) {
  const acknowledgedBy = window.prompt("Nom de la personne qui acquitte l’incident :", "Opérateur");
  if (!acknowledgedBy?.trim()) return;
  try {
    await api(`/incidents/${encodeURIComponent(incidentID)}/acknowledge`, {
      method: "POST",
      body: JSON.stringify({ acknowledged_by: acknowledgedBy.trim() }),
    });
    await refreshCurrent({ silent: true });
    showToast("Incident acquitté.");
  } catch (error) {
    showToast(error.message);
  }
}

async function cancelMaintenance(maintenanceID) {
  try {
    await api(`/maintenance-windows/${encodeURIComponent(maintenanceID)}`, { method: "DELETE" });
    const monitorID = state.settings.monitor.id;
    state.settings.maintenances = await api(`/monitors/${encodeURIComponent(monitorID)}/maintenance-windows`);
    renderSettings();
    showToast("Maintenance annulée.");
  } catch (error) {
    showToast(error.message);
  }
}

document.addEventListener("input", (event) => {
  if (event.target.matches('[data-control="search"]')) {
    state.search = event.target.value;
    renderRowsOnly();
  }
});

document.addEventListener("change", (event) => {
  if (event.target.matches('[data-control="sort"]')) {
    state.sort = event.target.value;
    renderRowsOnly();
  }
});

window.addEventListener("hashchange", () => {
  state.error = null;
  refreshCurrent();
  document.querySelector("#main-content").focus();
});

window.setInterval(() => refreshCurrent({ silent: true }), POLL_INTERVAL_MS);
refreshCurrent();
