package handlers

import (
	"net/http"
)

const (
	monitorAuditMonitorCreate        = "monitoring.monitor.create"
	monitorAuditMonitorUpdate        = "monitoring.monitor.update"
	monitorAuditMonitorDelete        = "monitoring.monitor.delete"
	monitorAuditMonitorPause         = "monitoring.monitor.pause"
	monitorAuditMonitorResume        = "monitoring.monitor.resume"
	monitorAuditMonitorCheckNow      = "monitoring.monitor.check_now"
	monitorAuditMonitorClone         = "monitoring.monitor.clone"
	monitorAuditMonitorPush          = "monitoring.monitor.push"
	monitorAuditMonitorEventsDelete  = "monitoring.monitor.events.delete"
	monitorAuditMonitorMetricsDelete = "monitoring.monitor.metrics.delete"

	monitorAuditSLAUpdate             = "monitoring.sla.update"
	monitorAuditSLAPolicyUpdate       = "monitoring.sla.policy.update"
	monitorAuditSettingsUpdate        = "monitoring.settings.update"
	monitorAuditCertsSettingsUpdate   = "monitoring.certs.settings.update"
	monitorAuditCertsNotifyTest       = "monitoring.certs.notify_test"
	monitorAuditCertsNotifyTestFailed = "monitoring.certs.notify_test.failed"

	monitorAuditMaintenanceCreate = "monitoring.maintenance.create"
	monitorAuditMaintenanceUpdate = "monitoring.maintenance.update"
	monitorAuditMaintenanceStop   = "monitoring.maintenance.stop"
	monitorAuditMaintenanceDelete = "monitoring.maintenance.delete"

	monitorAuditNotifChannelCreate   = "monitoring.notification.channel.create"
	monitorAuditNotifChannelUpdate   = "monitoring.notification.channel.update"
	monitorAuditNotifChannelDelete   = "monitoring.notification.channel.delete"
	monitorAuditNotifChannelTest     = "monitoring.notification.channel.test"
	monitorAuditNotifChannelReveal   = "monitoring.notification.channel.reveal_token"
	monitorAuditNotifChannelApplyAll = "monitoring.notification.channel.apply_all"
	monitorAuditNotifBindingsUpdate  = "monitoring.notification.bindings.update"
)

func (h *MonitoringHandler) audit(r *http.Request, action, details string) {
	if h == nil || h.audits == nil {
		return
	}
	_ = h.audits.Log(r.Context(), currentUsername(r), action, details)
}
