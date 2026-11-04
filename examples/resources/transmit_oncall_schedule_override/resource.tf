resource "transmit_oncall_schedule_override" "dana_vacation" {
  configuration_id = transmit_oncall_configuration.prod.id
  schedule_id      = transmit_oncall_schedule.primary.id
  starts_at        = "2026-12-24T00:00:00-05:00"
  ends_at          = "2026-12-27T00:00:00-05:00"
  responder_id     = transmit_oncall_responder.lee.id
  rotation_ids     = [transmit_oncall_schedule.primary.rotation_ids["Weekly"]]
}
