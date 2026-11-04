resource "transmit_oncall_schedule" "primary" {
  configuration_id = transmit_oncall_configuration.prod.id
  name             = "Primary"
  timezone         = "America/New_York"
  owner_team_id    = transmit_oncall_team.sre.id

  # Business hours hand off daily; nights and weekends weekly.
  rotation {
    name          = "Weekly"
    cadence       = "weekly"
    starts_at     = "2026-11-02T09:00:00-05:00"
    responder_ids = [transmit_oncall_responder.dana.id, transmit_oncall_responder.lee.id]
  }
}
