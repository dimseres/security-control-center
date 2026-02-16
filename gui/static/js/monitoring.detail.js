(() => {
  const els = {};
  const HOST_TARGET_TYPES = new Set(['tcp', 'ping', 'dns', 'docker', 'steam', 'gamedig', 'mqtt', 'kafka_producer', 'mssql', 'mysql', 'mongodb', 'radius', 'redis', 'tailscale_ping']);
  const detailState = {
    metricsRange: '1h',
    eventsRange: '1h',
    pollTimer: null,
    pollInFlight: false,
    chartMetrics: [],
    chartMeta: null,
    chartResizeObserver: null,
    chartResizeRaf: 0,
  };

  function bindDetail() {
    els.detail = document.getElementById('monitor-detail');
    els.empty = document.getElementById('monitor-empty');
    els.title = document.getElementById('monitor-detail-title');
    els.target = document.getElementById('monitor-detail-target');
    els.tags = document.getElementById('monitor-detail-tags');
    els.maintenance = document.getElementById('monitor-maintenance-info');
    els.detailAlert = document.getElementById('monitor-detail-alert');
    els.dot = document.getElementById('monitor-status-dot');
    els.strip = document.getElementById('monitor-status-strip');
    els.stats = document.getElementById('monitor-stats');
    els.chart = document.getElementById('monitor-latency-chart');
    els.latencyRange = document.getElementById('monitor-latency-range');
    els.events = document.getElementById('monitor-events-list');
    els.pause = document.getElementById('monitor-pause-toggle');
    els.edit = document.getElementById('monitor-edit');
    els.clone = document.getElementById('monitor-clone');
    els.remove = document.getElementById('monitor-delete');
    els.eventsRange = document.getElementById('monitor-events-range');
    els.clearStats = document.getElementById('monitor-events-clear');
    ensureChartTooltip();
    bindChartAutoResize();

    if (els.latencyRange) {
      els.latencyRange.value = detailState.metricsRange;
      els.latencyRange.addEventListener('change', () => {
        detailState.metricsRange = els.latencyRange.value;
        const id = MonitoringPage.state.selectedId;
        if (id) loadDetail(id);
      });
    }
    if (els.eventsRange) {
      els.eventsRange.value = detailState.eventsRange;
      els.eventsRange.addEventListener('change', () => {
        detailState.eventsRange = els.eventsRange.value;
        const id = MonitoringPage.state.selectedId;
        if (id) loadDetail(id);
      });
      if (!MonitoringPage.hasPermission('monitoring.events.view')) {
        els.eventsRange.disabled = true;
      }
    }
    if (els.clearStats) {
      const canManage = MonitoringPage.hasPermission('monitoring.manage');
      els.clearStats.disabled = !canManage;
      els.clearStats.addEventListener('change', async () => {
        const action = els.clearStats.value;
        if (!action) return;
        const mon = MonitoringPage.selectedMonitor();
        if (!mon) return;
        const labelKey = action === 'events'
          ? 'monitoring.events.clearConfirmEvents'
          : 'monitoring.events.clearConfirmMetrics';
        const confirmed = window.confirm(MonitoringPage.t(labelKey));
        if (!confirmed) {
          els.clearStats.value = '';
          return;
        }
        try {
          if (action === 'events') {
            await Api.del(`/api/monitoring/monitors/${mon.id}/events`);
          } else {
            await Api.del(`/api/monitoring/monitors/${mon.id}/metrics`);
          }
          await loadDetail(mon.id);
          MonitoringPage.refreshEventsCenter?.();
        } catch (err) {
          console.error('clear stats', err);
        } finally {
          els.clearStats.value = '';
        }
      });
    }
    if (els.pause) els.pause.addEventListener('click', handlePause);
    if (els.edit) els.edit.addEventListener('click', () => MonitoringPage.openMonitorModal?.(MonitoringPage.selectedMonitor()));
    if (els.clone) els.clone.addEventListener('click', handleClone);
    if (els.remove) els.remove.addEventListener('click', handleDelete);
    document.addEventListener('visibilitychange', () => {
      if (document.hidden) return;
      const id = MonitoringPage.state.selectedId;
      if (id) loadDetail(id);
    });
  }

  async function loadDetail(id) {
    if (!id) return;
    renderChartLoading();
    try {
      const canEvents = MonitoringPage.hasPermission('monitoring.events.view');
      const canMaintenance = MonitoringPage.hasPermission('monitoring.maintenance.view');
      const requests = [
        Api.get(`/api/monitoring/monitors/${id}`),
        Api.get(`/api/monitoring/monitors/${id}/state`),
        Api.get(`/api/monitoring/monitors/${id}/metrics?range=${detailState.metricsRange}`),
        canEvents
          ? Api.get(`/api/monitoring/monitors/${id}/events?range=${detailState.eventsRange}`)
          : Promise.resolve({ items: [] }),
        canMaintenance
          ? Api.get(`/api/monitoring/maintenance?active=true&monitor_id=${id}`)
          : Promise.resolve({ items: [] }),
      ];
      const [mon, state, metrics, events, maintenance] = await Promise.all(requests);
      const current = MonitoringPage.state.monitors.find(m => m.id === id);
      if (current) Object.assign(current, mon);
      const metricsItems = metrics.items || [];
      renderDetail(mon, state, metricsItems, events.items || [], maintenance.items || [], {
        from: metrics.from || null,
        to: metrics.to || null,
      });
      return { mon, state, metrics: metricsItems, events: events.items || [], maintenance: maintenance.items || [] };
    } catch (err) {
      console.error('monitor detail', err);
      const msg = (err?.message || '').trim();
      if (msg === 'common.notFound' || msg === 'not found') {
        if (MonitoringPage.state.selectedId === id) {
          MonitoringPage.state.selectedId = null;
        }
        stopDetailRefresh();
        await MonitoringPage.loadMonitors?.();
        return null;
      }
      const fallback = MonitoringPage.state.monitors.find(m => m.id === id);
      if (fallback) scheduleDetailRefresh(fallback);
      return null;
    }
  }

  function renderDetail(mon, state, metrics, events, maintenance, metricsMeta) {
    if (!els.detail || !els.empty) return;
    if (!mon) {
      clearDetail();
      return;
    }
    els.empty.hidden = true;
    els.detail.hidden = false;
    if (els.title) els.title.textContent = mon.name || `#${mon.id}`;
    if (els.target) els.target.textContent = HOST_TARGET_TYPES.has((mon.type || '').toLowerCase())
      ? (mon.port ? `${mon.host}:${mon.port}` : (mon.host || '-'))
      : (mon.url || mon.host || '-');
    renderMonitorTags(mon?.tags || []);
    if (els.dot) els.dot.className = `status-dot ${statusClass(state?.status || mon.status)}`;
    const current = MonitoringPage.state.monitors.find(m => m.id === mon.id);
    if (current) {
      current.status = state?.status || mon.status || current.status;
      current.last_checked_at = state?.last_checked_at || current.last_checked_at || null;
      current.last_status_code = state?.last_status_code ?? current.last_status_code ?? null;
      current.last_error = state?.last_error || current.last_error || '';
      current.last_latency_ms = state?.last_latency_ms ?? current.last_latency_ms ?? null;
      MonitoringPage.renderMonitorList?.();
    }
    renderMaintenanceInfo(mon, state, maintenance);
    renderStatusStrip(metrics);
    renderStats(mon, state);
    detailState.chartMetrics = Array.isArray(metrics) ? metrics.slice() : [];
    detailState.chartMeta = metricsMeta || null;
    renderLatencyChart(metrics, metricsMeta || null);
    renderEvents(events);
    updateActionLabels(mon);
    toggleActionAccess();
    scheduleDetailRefresh(mon);
  }

  function clearDetail() {
    stopDetailRefresh();
    if (els.detail) els.detail.hidden = true;
    if (els.empty) els.empty.hidden = (MonitoringPage.state.monitors || []).length === 0;
    if (els.tags) {
      els.tags.hidden = true;
      els.tags.innerHTML = '';
    }
  }

  function renderMonitorTags(tags) {
    if (!els.tags) return;
    const values = Array.isArray(tags) ? tags.filter(Boolean) : [];
    els.tags.innerHTML = '';
    if (!values.length) {
      els.tags.hidden = true;
      return;
    }
    values.forEach((tagCode) => {
      const chip = document.createElement('span');
      chip.className = 'monitor-item-tag';
      chip.textContent = MonitoringPage.tagLabel ? MonitoringPage.tagLabel(tagCode) : tagCode;
      chip.title = chip.textContent;
      els.tags.appendChild(chip);
    });
    els.tags.hidden = false;
  }

  function renderStatusStrip(metrics) {
    if (!els.strip) return;
    els.strip.innerHTML = '';
    const slice = metrics.slice(-50);
    slice.forEach(m => {
      const bar = document.createElement('span');
      bar.className = m.ok ? 'up' : 'down';
      els.strip.appendChild(bar);
    });
    if (!slice.length) {
      const bar = document.createElement('span');
      bar.className = 'paused';
      els.strip.appendChild(bar);
    }
  }

  function renderStats(mon, state) {
    if (!els.stats) return;
    const lastCode = state?.last_status_code ? `${state.last_status_code}` : '-';
    const lastErr = state?.last_error ? MonitoringPage.sanitizeErrorMessage(state.last_error) : '';
    els.stats.innerHTML = '';
    els.stats.appendChild(statCard(MonitoringPage.t('monitoring.stats.current'), lastErr ? lastErr : lastCode));
    els.stats.appendChild(statCard(MonitoringPage.t('monitoring.stats.avg24h'), MonitoringPage.formatLatency(state?.avg_latency_24h)));
    els.stats.appendChild(statCard(MonitoringPage.t('monitoring.stats.uptime24h'), MonitoringPage.formatUptime(state?.uptime_24h)));
    els.stats.appendChild(statCard(MonitoringPage.t('monitoring.stats.uptime30d'), MonitoringPage.formatUptime(state?.uptime_30d)));
    if (mon.sla_target_pct) {
      const slaOk = (state?.uptime_30d || 0) >= mon.sla_target_pct;
      const label = MonitoringPage.t('monitoring.stats.sla');
      const value = slaOk
        ? MonitoringPage.t('monitoring.sla.ok')
        : MonitoringPage.t('monitoring.sla.violated');
      els.stats.appendChild(statCard(label, value));
    }
  }

  function pointsFromMetrics(metrics, scaleX) {
    return metrics.map((m, idx) => {
      const tsRaw = m.timestamp || m.ts;
      const tsMs = Number.isFinite(Date.parse(tsRaw)) ? Date.parse(tsRaw) : Date.now();
      return {
        x: scaleX(tsMs, idx),
        ok: !!m.ok,
        ts: tsRaw,
        tsMs,
        latency: m.latency_ms || 0,
        statusCode: m.status_code ?? m.statusCode ?? null,
        error: m.error || '',
      };
    });
  }

  function renderXAxisLabels(svg, domainFrom, domainTo, scaleX, pad, height, intervalMs, rangeKey) {
    if (!intervalMs || !Number.isFinite(domainFrom) || !Number.isFinite(domainTo) || domainTo <= domainFrom) return;
    const ticks = [];
    const start = Math.floor(domainFrom / intervalMs) * intervalMs;
    for (let ts = start; ts <= domainTo + 1; ts += intervalMs) {
      ticks.push(ts);
    }
    ticks.forEach((ts, n) => {
      if (ts < domainFrom || ts > domainTo) return;
      const x = scaleX(ts, n);
      const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      label.setAttribute('x', `${x}`);
      label.setAttribute('y', `${height - 8}`);
      label.setAttribute('font-size', '11');
      label.setAttribute('fill', 'rgba(255, 255, 255, 0.62)');
      if (n === 0) {
        label.setAttribute('text-anchor', 'start');
      } else if (n === ticks.length - 1) {
        label.setAttribute('text-anchor', 'end');
      } else {
        label.setAttribute('text-anchor', 'middle');
      }
      label.textContent = formatXAxisTick(ts, rangeKey);
      svg.appendChild(label);

      const vline = document.createElementNS('http://www.w3.org/2000/svg', 'line');
      vline.setAttribute('x1', `${x}`);
      vline.setAttribute('x2', `${x}`);
      vline.setAttribute('y1', `${pad.top}`);
      vline.setAttribute('y2', `${height - pad.bottom}`);
      vline.setAttribute('stroke', 'rgba(255, 255, 255, 0.06)');
      vline.setAttribute('stroke-width', '1');
      svg.appendChild(vline);
    });
  }

  function ensureChartTooltip() {
    if (!els.chart) return;
    if (els.chartTip && els.chartTip.parentElement) return;
    const tip = document.createElement('div');
    tip.className = 'monitoring-chart-tooltip';
    tip.hidden = true;
    els.chart.appendChild(tip);
    els.chartTip = tip;
  }

  function bindChartAutoResize() {
    if (!els.chart || detailState.chartResizeObserver || typeof ResizeObserver === 'undefined') return;
    detailState.chartResizeObserver = new ResizeObserver(() => {
      if (detailState.chartResizeRaf) {
        window.cancelAnimationFrame(detailState.chartResizeRaf);
      }
      detailState.chartResizeRaf = window.requestAnimationFrame(() => {
        detailState.chartResizeRaf = 0;
        rerenderChartFromCache();
      });
    });
    detailState.chartResizeObserver.observe(els.chart);
  }

  function rerenderChartFromCache() {
    if (!els.chart) return;
    if (els.detail && els.detail.hidden) return;
    if (!Array.isArray(detailState.chartMetrics) || !detailState.chartMetrics.length) return;
    renderLatencyChart(detailState.chartMetrics, detailState.chartMeta || null);
  }

  function renderChartLoading() {
    if (!els.chart) return;
    els.chart.innerHTML = '';
    const loading = document.createElement('div');
    loading.className = 'muted';
    const text = MonitoringPage.t('common.loading');
    loading.textContent = text && text !== 'common.loading' ? text : '...';
    els.chart.appendChild(loading);
  }

  function showChartTooltip(text) {
    if (!els.chartTip) return;
    els.chartTip.textContent = text;
    els.chartTip.hidden = false;
  }

  function moveChartTooltip(evt) {
    if (!els.chartTip || els.chartTip.hidden || !els.chart) return;
    const rect = els.chart.getBoundingClientRect();
    const x = Math.max(8, Math.min(rect.width - 8, evt.clientX - rect.left + 10));
    const y = Math.max(8, Math.min(rect.height - 8, evt.clientY - rect.top + 10));
    els.chartTip.style.left = `${x}px`;
    els.chartTip.style.top = `${y}px`;
  }

  function hideChartTooltip() {
    if (!els.chartTip) return;
    els.chartTip.hidden = true;
  }

  function renderLatencyChart(metrics, metricsMeta) {
    if (!els.chart) return;
    els.chart.innerHTML = '';
    ensureChartTooltip();
    if (!metrics.length) {
      els.chart.textContent = MonitoringPage.t('monitoring.noMetrics');
      return;
    }
    const toTs = Number.isFinite(Date.parse(metricsMeta?.to || '')) ? Date.parse(metricsMeta.to) : Date.now();
    const fromTs = Number.isFinite(Date.parse(metricsMeta?.from || '')) ? Date.parse(metricsMeta.from) : (toTs - inferRangeMs(detailState.metricsRange));
    const domainFrom = Math.min(fromTs, toTs - 1000);
    const domainTo = Math.max(toTs, domainFrom + 1000);
    const bucketMs = aggregationBucketMs(detailState.metricsRange);
    const plottedMetrics = bucketMs > 0
      ? aggregateMetrics(metrics, bucketMs, domainFrom, domainTo)
      : metrics;

    const upValues = plottedMetrics.filter(m => !!m.ok).map(m => Math.max(0, m.latency_ms || 0));
    const maxUp = Math.max(...upValues, 0);
    const scaleMax = computeLatencyScaleMax(maxUp);
    const step = niceStep(Math.max(1, scaleMax / 5));
    const maxTick = Math.max(step, Math.ceil(scaleMax / step) * step);
    const width = Math.max(640, Math.floor(els.chart.getBoundingClientRect().width || els.chart.clientWidth || 980));
    const height = 260;
    const pad = { left: 54, right: 14, top: 14, bottom: 34 };
    const chartWidth = width - pad.left - pad.right;
    const chartHeight = height - pad.top - pad.bottom;
    const scaleX = (tsMs, idx) => {
      if (Number.isFinite(tsMs)) {
        const ratio = (tsMs - domainFrom) / Math.max(1, (domainTo - domainFrom));
        return pad.left + Math.max(0, Math.min(1, ratio)) * chartWidth;
      }
      return pad.left + (idx / Math.max(plottedMetrics.length - 1, 1)) * chartWidth;
    };
    const scaleY = (val) => pad.top + (1 - (val / maxTick)) * chartHeight;
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', `0 0 ${width} ${height}`);
    svg.setAttribute('preserveAspectRatio', 'none');

    const ticks = [];
    for (let v = 0; v <= maxTick; v += step) ticks.push(v);
    ticks.forEach(val => {
      const y = scaleY(val);
      const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
      line.setAttribute('x1', pad.left);
      line.setAttribute('x2', width - pad.right);
      line.setAttribute('y1', y);
      line.setAttribute('y2', y);
      line.setAttribute('stroke', 'rgba(255, 255, 255, 0.08)');
      line.setAttribute('stroke-width', '1');
      svg.appendChild(line);
      const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      label.setAttribute('x', `${pad.left - 8}`);
      label.setAttribute('y', `${y + 4}`);
      label.setAttribute('text-anchor', 'end');
      label.setAttribute('font-size', '12');
      label.setAttribute('fill', 'rgba(255, 255, 255, 0.62)');
      label.textContent = `${val}`;
      svg.appendChild(label);
    });
    const hoverPoints = pointsFromMetrics(metrics, scaleX).map(pt => ({
      ...pt,
      y: scaleY(Math.max(0, Math.min(maxTick, pt.latency))),
    }));
    const points = pointsFromMetrics(plottedMetrics, scaleX).map(pt => ({
      ...pt,
      y: scaleY(Math.max(0, Math.min(maxTick, pt.latency))),
    }));
    renderXAxisLabels(
      svg,
      domainFrom,
      domainTo,
      scaleX,
      pad,
      height,
      rangeLabelIntervalMs(detailState.metricsRange),
      detailState.metricsRange
    );

    renderDownBackgrounds(svg, hoverPoints, pad, height);
    const upSegments = [];
    let segment = [];
    points.forEach((pt) => {
      if (pt.ok) {
        segment.push(pt);
      } else if (segment.length) {
        upSegments.push(segment);
        segment = [];
      }
    });
    if (segment.length) upSegments.push(segment);

    upSegments.forEach((seg) => {
      if (seg.length < 2) return;
      const poly = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
      poly.setAttribute('fill', 'none');
      poly.setAttribute('stroke', '#2dd27b');
      poly.setAttribute('stroke-width', '3');
      poly.setAttribute('stroke-linecap', 'round');
      poly.setAttribute('stroke-linejoin', 'round');
      poly.setAttribute('points', seg.map(p => `${p.x.toFixed(2)},${p.y.toFixed(2)}`).join(' '));
      svg.appendChild(poly);
    });

    const upPoints = points.filter(pt => pt.ok);
    upPoints.forEach((pt, idx, arr) => renderPoint(svg, pt, idx, arr.length));
    bindChartHover(svg, hoverPoints, pad, width, height);

    els.chart.appendChild(svg);
  }

  function inferRangeMs(rangeKey) {
    switch ((rangeKey || '').toLowerCase()) {
      case '1h': return 60 * 60 * 1000;
      case '3h': return 3 * 60 * 60 * 1000;
      case '6h': return 6 * 60 * 60 * 1000;
      case '24h': return 24 * 60 * 60 * 1000;
      case '7d': return 7 * 24 * 60 * 60 * 1000;
      case '30d': return 30 * 24 * 60 * 60 * 1000;
      default: return 24 * 60 * 60 * 1000;
    }
  }

  function rangeLabelIntervalMs(rangeKey) {
    switch ((rangeKey || '').toLowerCase()) {
      case '1h': return 5 * 60 * 1000;
      case '3h': return 15 * 60 * 1000;
      case '6h': return 30 * 60 * 1000;
      case '24h': return 60 * 60 * 1000;
      case '7d': return 6 * 60 * 60 * 1000;
      case '30d': return 24 * 60 * 60 * 1000;
      default: return 60 * 60 * 1000;
    }
  }

  function aggregationBucketMs(rangeKey) {
    switch ((rangeKey || '').toLowerCase()) {
      case '24h': return 60 * 60 * 1000;
      case '7d': return 6 * 60 * 60 * 1000;
      case '30d': return 24 * 60 * 60 * 1000;
      default: return 0;
    }
  }

  function aggregateMetrics(metrics, bucketMs, fromMs, toMs) {
    if (!Array.isArray(metrics) || !metrics.length || !bucketMs) return metrics || [];
    const buckets = new Map();
    metrics.forEach((m) => {
      const tsRaw = m.timestamp || m.ts;
      const tsMs = Number.isFinite(Date.parse(tsRaw)) ? Date.parse(tsRaw) : 0;
      if (!tsMs || tsMs < fromMs || tsMs > toMs) return;
      const key = Math.floor((tsMs - fromMs) / bucketMs);
      if (!buckets.has(key)) {
        buckets.set(key, {
          from: fromMs + key * bucketMs,
          to: fromMs + (key + 1) * bucketMs,
          upCount: 0,
          upLatencySum: 0,
          lastStatusCode: null,
          lastError: '',
        });
      }
      const b = buckets.get(key);
      if (m.ok) {
        b.upCount += 1;
        b.upLatencySum += Math.max(0, m.latency_ms || 0);
      }
      b.lastStatusCode = m.status_code ?? m.statusCode ?? b.lastStatusCode;
      if (m.error) b.lastError = m.error;
    });
    return Array.from(buckets.values())
      .sort((a, b) => a.from - b.from)
      .map((b) => {
        const mid = new Date(Math.floor((b.from + b.to) / 2)).toISOString();
        return {
          timestamp: mid,
          latency_ms: b.upCount > 0 ? Math.round(b.upLatencySum / b.upCount) : 0,
          ok: b.upCount > 0,
          status_code: b.lastStatusCode,
          error: b.lastError,
        };
      });
  }

  function formatXAxisTick(tsMs, rangeKey) {
    const date = new Date(tsMs);
    const lang = (window.localStorage?.getItem('lang') || 'ru').toLowerCase();
    const locale = lang === 'ru' ? 'ru-RU' : 'en-US';
    const key = (rangeKey || '').toLowerCase();
    if (key === '7d' || key === '30d') {
      return date.toLocaleDateString(locale, { day: '2-digit', month: '2-digit' });
    }
    return date.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  function renderDownBackgrounds(svg, points, pad, height) {
    if (!Array.isArray(points) || !points.length) return;
    const top = pad.top;
    const bottom = height - pad.bottom;
    points.forEach((pt, idx) => {
      if (pt.ok) return;
      const prevX = idx > 0 ? points[idx - 1].x : pt.x;
      const nextX = idx < points.length - 1 ? points[idx + 1].x : pt.x;
      const startX = idx > 0 ? (prevX + pt.x) / 2 : Math.max(pad.left, pt.x - Math.max(6, (nextX - pt.x) / 2));
      const endX = idx < points.length - 1 ? (pt.x + nextX) / 2 : Math.min(points[points.length - 1].x, pt.x + Math.max(6, (pt.x - prevX) / 2));
      const width = Math.max(2, endX - startX);
      const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
      rect.setAttribute('x', `${startX}`);
      rect.setAttribute('y', `${top}`);
      rect.setAttribute('width', `${width}`);
      rect.setAttribute('height', `${bottom - top}`);
      rect.setAttribute('fill', 'rgba(255, 77, 79, 0.20)');
      svg.appendChild(rect);
    });
  }

  function renderPoint(svg, pt, idx, total) {
    const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
    circle.setAttribute('cx', pt.x.toFixed(2));
    circle.setAttribute('cy', pt.y.toFixed(2));
    circle.setAttribute('r', (idx === 0 || idx === total - 1) ? '4' : '2.5');
    circle.setAttribute('fill', pt.ok ? '#2dd27b' : '#ff6b6b');
    svg.appendChild(circle);
  }

  function bindChartHover(svg, points, pad, width, height) {
    if (!Array.isArray(points) || !points.length) return;
    const overlay = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
    overlay.setAttribute('x', `${pad.left}`);
    overlay.setAttribute('y', `${pad.top}`);
    overlay.setAttribute('width', `${Math.max(0, width - pad.left - pad.right)}`);
    overlay.setAttribute('height', `${Math.max(0, height - pad.top - pad.bottom)}`);
    overlay.setAttribute('fill', 'transparent');
    overlay.style.pointerEvents = 'all';
    overlay.addEventListener('mousemove', (e) => {
      const rect = svg.getBoundingClientRect();
      const mouseX = e.clientX - rect.left;
      const nearest = nearestPoint(points, mouseX);
      if (!nearest) return;
      showChartTooltip(pointTooltipText(nearest));
      moveChartTooltip(e);
    });
    overlay.addEventListener('mouseleave', hideChartTooltip);
    svg.appendChild(overlay);
  }

  function nearestPoint(points, x) {
    let best = null;
    let bestDist = Number.POSITIVE_INFINITY;
    for (let i = 0; i < points.length; i += 1) {
      const d = Math.abs(points[i].x - x);
      if (d < bestDist) {
        bestDist = d;
        best = points[i];
      }
    }
    return best;
  }

  function pointTooltipText(pt) {
    const statusText = pt.ok ? MonitoringPage.t('monitoring.status.up') : MonitoringPage.t('monitoring.status.down');
    const codeText = pt.statusCode ? `HTTP ${pt.statusCode}` : '-';
    const errText = pt.error ? MonitoringPage.sanitizeErrorMessage(pt.error) : '-';
    return [
      `${MonitoringPage.t('monitoring.tooltip.time')}: ${MonitoringPage.formatDate(pt.ts)}`,
      `${MonitoringPage.t('monitoring.tooltip.latency')}: ${MonitoringPage.formatLatency(pt.latency)}`,
      `${MonitoringPage.t('monitoring.tooltip.status')}: ${statusText}`,
      `${MonitoringPage.t('monitoring.tooltip.code')}: ${codeText}`,
      `${MonitoringPage.t('monitoring.tooltip.error')}: ${errText}`,
    ].join('\n');
  }

  function niceStep(raw) {
    const value = Math.max(1, Number(raw) || 1);
    const pow = Math.pow(10, Math.floor(Math.log10(value)));
    const base = value / pow;
    if (base <= 1) return 1 * pow;
    if (base <= 2) return 2 * pow;
    if (base <= 5) return 5 * pow;
    return 10 * pow;
  }

  function computeLatencyScaleMax(maxUp) {
    const value = Math.max(0, Number(maxUp) || 0);
    if (value <= 10) return 50;
    if (value <= 50) return 100;
    if (value <= 100) return 200;
    let limit = 200;
    while (limit < value) {
      limit *= 2;
    }
    return limit;
  }

  function stopDetailRefresh() {
    if (detailState.pollTimer) {
      window.clearTimeout(detailState.pollTimer);
      detailState.pollTimer = null;
    }
  }

  function scheduleDetailRefresh(mon) {
    stopDetailRefresh();
    if (!mon || !mon.id || mon.is_paused || !mon.is_active) return;
    const intervalSec = Number(mon.interval_sec) || 30;
    const waitMs = Math.min(Math.max(intervalSec * 1000, 3000), 60000);
    detailState.pollTimer = window.setTimeout(async () => {
      if (document.hidden) {
        scheduleDetailRefresh(mon);
        return;
      }
      if (detailState.pollInFlight) {
        scheduleDetailRefresh(mon);
        return;
      }
      detailState.pollInFlight = true;
      try {
        await loadDetail(mon.id);
      } finally {
        detailState.pollInFlight = false;
      }
    }, waitMs);
  }

  function renderEvents(events) {
    if (!els.events) return;
    els.events.innerHTML = '';
    if (!events.length) {
      const empty = document.createElement('div');
      empty.className = 'muted';
      empty.textContent = MonitoringPage.t('monitoring.noEvents');
      els.events.appendChild(empty);
      return;
    }
    events.forEach(ev => {
      const row = document.createElement('div');
      row.className = `monitoring-event ${statusClass(ev.event_type)}`;
      row.innerHTML = `
        <div>
          <div>${statusLabel(ev.event_type)}</div>
          <div class="event-meta">${MonitoringPage.sanitizeErrorMessage(ev.message || '')}</div>
        </div>
        <div class="event-meta">${MonitoringPage.formatDate(ev.ts)}</div>
      `;
      els.events.appendChild(row);
    });
  }

  function renderMaintenanceInfo(mon, state, items) {
    if (!els.maintenance) return;
    const active = state?.maintenance_active || (state?.status || '').toLowerCase() === 'maintenance';
    if (!active || !items || !items.length) {
      els.maintenance.hidden = true;
      return;
    }
    const tags = mon?.tags || [];
    const applicable = items.filter(item => appliesToMonitor(item, mon?.id, tags));
    if (!applicable.length) {
      els.maintenance.hidden = true;
      return;
    }
    const next = applicable.slice().sort((a, b) => new Date(a.ends_at) - new Date(b.ends_at))[0];
    const until = next?.ends_at ? MonitoringPage.formatDate(next.ends_at) : '-';
    els.maintenance.textContent = `${MonitoringPage.t('monitoring.maintenance.activeUntil')} ${until}`;
    els.maintenance.hidden = false;
  }

  function appliesToMonitor(item, id, tags) {
    if (!item) return false;
    if (item.monitor_id && item.monitor_id !== id) return false;
    const itemTags = item.tags || [];
    if (!itemTags.length) return true;
    return itemTags.some(tag => tags.includes(tag));
  }

  function statCard(label, value) {
    const card = document.createElement('div');
    card.className = 'monitoring-stat';
    card.innerHTML = `<div class="label">${label}</div><div class="value">${value || '-'}</div>`;
    return card;
  }

  function updateActionLabels(mon) {
    if (!els.pause) return;
    const paused = !!mon.is_paused;
    els.pause.textContent = paused
      ? MonitoringPage.t('monitoring.actions.resume')
      : MonitoringPage.t('monitoring.actions.pause');
  }

  function toggleActionAccess() {
    const canManage = MonitoringPage.hasPermission('monitoring.manage');
    [els.pause, els.edit, els.clone, els.remove].forEach(btn => {
      if (!btn) return;
      btn.disabled = !canManage;
      btn.classList.toggle('disabled', !canManage);
    });
  }

  async function handlePause() {
    const mon = MonitoringPage.selectedMonitor();
    if (!mon) return;
    const paused = !!mon.is_paused;
    const action = paused ? 'resume' : 'pause';
    try {
      clearDetailAlert();
      await Api.post(`/api/monitoring/monitors/${mon.id}/${action}`);
      await MonitoringPage.loadMonitors?.();
      await loadDetail(mon.id);
    } catch (err) {
      console.error('pause', err);
      showDetailAlert(MonitoringPage.sanitizeErrorMessage(err.message || err));
      await loadDetail(mon.id);
    }
  }

  async function handleClone() {
    const mon = MonitoringPage.selectedMonitor();
    if (!mon) return;
    try {
      const res = await Api.post(`/api/monitoring/monitors/${mon.id}/clone`, {});
      await MonitoringPage.loadMonitors?.();
      const nextId = res?.id || res?.monitor_id || null;
      if (nextId) {
        MonitoringPage.state.selectedId = nextId;
        await loadDetail(nextId);
      }
    } catch (err) {
      console.error('clone', err);
    }
  }

  async function handleDelete() {
    const mon = MonitoringPage.selectedMonitor();
    if (!mon) return;
    const confirmed = window.confirm(MonitoringPage.t('monitoring.confirmDelete'));
    if (!confirmed) return;
    try {
      await Api.del(`/api/monitoring/monitors/${mon.id}`);
      MonitoringPage.state.selectedId = null;
      await MonitoringPage.loadMonitors?.();
      clearDetail();
    } catch (err) {
      console.error('delete', err);
    }
  }

  function showDetailAlert(message, success = false) {
    if (!els.detailAlert || !message) return;
    MonitoringPage.showAlert(els.detailAlert, message, success);
  }

  function clearDetailAlert() {
    if (!els.detailAlert) return;
    MonitoringPage.hideAlert(els.detailAlert);
  }

  function statusClass(status) {
    const val = (status || '').toLowerCase();
    if (val === 'up') return 'up';
    if (val === 'paused') return 'paused';
    if (val === 'maintenance' || val === 'maintenance_start' || val === 'maintenance_end') return 'maintenance';
    return 'down';
  }

  function statusLabel(status) {
    const val = (status || '').toLowerCase();
    if (val === 'maintenance_start') return MonitoringPage.t('monitoring.event.maintenanceStart');
    if (val === 'maintenance_end') return MonitoringPage.t('monitoring.event.maintenanceEnd');
    if (val === 'tls_expiring') return MonitoringPage.t('monitoring.event.tlsExpiring');
    const key = `monitoring.status.${val}`;
    return MonitoringPage.t(key);
  }

  if (typeof MonitoringPage !== 'undefined') {
    MonitoringPage.bindDetail = bindDetail;
    MonitoringPage.loadDetail = loadDetail;
    MonitoringPage.clearDetail = clearDetail;
  }
})();
