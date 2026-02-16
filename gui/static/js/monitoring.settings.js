(() => {
  const els = {};

  function bindSettings() {
    els.alert = document.getElementById('monitoring-settings-alert');
    els.form = document.getElementById('monitoring-settings-form');
    els.defaultsForm = document.getElementById('monitoring-defaults-form');
    els.save = document.getElementById('monitoring-settings-save');
    els.retention = document.getElementById('monitoring-retention');
    els.maxConcurrent = document.getElementById('monitoring-max-concurrent');
    els.defaultTimeout = document.getElementById('monitoring-default-timeout');
    els.defaultInterval = document.getElementById('monitoring-default-interval');
    els.defaultRetries = document.getElementById('monitoring-default-retries');
    els.defaultRetryInterval = document.getElementById('monitoring-default-retry-interval');
    els.defaultSla = document.getElementById('monitoring-default-sla');
    els.engineEnabled = document.getElementById('monitoring-engine-enabled');
    els.allowPrivate = document.getElementById('monitoring-allow-private');
    els.tlsRefresh = document.getElementById('monitoring-tls-refresh');
    els.tlsExpiring = document.getElementById('monitoring-tls-expiring');
    els.notifySuppress = document.getElementById('monitoring-notify-suppress');
    els.notifyRepeat = document.getElementById('monitoring-notify-repeat');
    els.notifyMaintenance = document.getElementById('monitoring-notify-maintenance');
    els.autoTLSIncident = document.getElementById('monitoring-auto-tls-incident');
    els.autoTLSIncidentDays = document.getElementById('monitoring-auto-tls-incident-days');

    if (!MonitoringPage.hasPermission('monitoring.settings.manage')) {
      const card = els.form?.closest('.card');
      if (card) card.hidden = true;
      const defaultsCard = els.defaultsForm?.closest('.card');
      if (defaultsCard) defaultsCard.hidden = true;
      return;
    }
    if (els.save) {
      els.save.addEventListener('click', async () => {
        await saveSettings();
      });
    }
    loadSettings();
  }

  async function loadSettings() {
    try {
      const res = await Api.get('/api/monitoring/settings');
      MonitoringPage.state.settings = res;
      renderSettings(res);
    } catch (err) {
      console.error('monitoring settings', err);
    }
  }

  function renderSettings(settings) {
    if (!settings) return;
    if (els.retention) els.retention.value = settings.retention_days || 30;
    if (els.maxConcurrent) els.maxConcurrent.value = settings.max_concurrent_checks || 10;
    if (els.defaultTimeout) els.defaultTimeout.value = settings.default_timeout_sec || 5;
    if (els.defaultInterval) els.defaultInterval.value = settings.default_interval_sec || 60;
    if (els.defaultRetries) els.defaultRetries.value = settings.default_retries ?? 0;
    if (els.defaultRetryInterval) els.defaultRetryInterval.value = settings.default_retry_interval_sec || 5;
    if (els.defaultSla) els.defaultSla.value = settings.default_sla_target_pct || 90;
    if (els.engineEnabled) els.engineEnabled.checked = !!settings.engine_enabled;
    if (els.allowPrivate) els.allowPrivate.checked = !!settings.allow_private_networks;
    if (els.tlsRefresh) els.tlsRefresh.value = settings.tls_refresh_hours || 24;
    if (els.tlsExpiring) els.tlsExpiring.value = settings.tls_expiring_days || 30;
    if (els.notifySuppress) els.notifySuppress.value = settings.notify_suppress_minutes || 5;
    if (els.notifyRepeat) els.notifyRepeat.value = settings.notify_repeat_down_minutes || 30;
    if (els.notifyMaintenance) els.notifyMaintenance.checked = !!settings.notify_maintenance;
    if (els.autoTLSIncident) els.autoTLSIncident.checked = !!settings.auto_tls_incident;
    if (els.autoTLSIncidentDays) els.autoTLSIncidentDays.value = settings.auto_tls_incident_days || 14;
  }

  async function saveSettings() {
    if (!MonitoringPage.hasPermission('monitoring.settings.manage')) return;
    MonitoringPage.hideAlert(els.alert);
    const payload = {
      retention_days: parseInt(els.retention.value, 10) || 0,
      max_concurrent_checks: parseInt(els.maxConcurrent.value, 10) || 0,
      default_timeout_sec: parseInt(els.defaultTimeout.value, 10) || 0,
      default_interval_sec: parseInt(els.defaultInterval.value, 10) || 0,
      default_retries: parseInt(els.defaultRetries?.value, 10) || 0,
      default_retry_interval_sec: parseInt(els.defaultRetryInterval?.value, 10) || 0,
      default_sla_target_pct: parseFloat(els.defaultSla?.value) || 0,
      engine_enabled: !!els.engineEnabled.checked,
      allow_private_networks: !!els.allowPrivate.checked,
      tls_refresh_hours: parseInt(els.tlsRefresh.value, 10) || 0,
      tls_expiring_days: parseInt(els.tlsExpiring.value, 10) || 0,
      notify_suppress_minutes: parseInt(els.notifySuppress.value, 10) || 0,
      notify_repeat_down_minutes: parseInt(els.notifyRepeat.value, 10) || 0,
      notify_maintenance: !!els.notifyMaintenance.checked,
      auto_tls_incident: !!els.autoTLSIncident?.checked,
      auto_tls_incident_days: parseInt(els.autoTLSIncidentDays?.value, 10) || 0,
    };
    try {
      const res = await Api.put('/api/monitoring/settings', payload);
      MonitoringPage.state.settings = res;
      renderSettings(res);
      MonitoringPage.showAlert(els.alert, MonitoringPage.t('monitoring.settings.saved'), true);
    } catch (err) {
      MonitoringPage.showAlert(els.alert, MonitoringPage.sanitizeErrorMessage(err.message || err), false);
    }
  }

  if (typeof MonitoringPage !== 'undefined') {
    MonitoringPage.bindSettings = bindSettings;
  }
})();
